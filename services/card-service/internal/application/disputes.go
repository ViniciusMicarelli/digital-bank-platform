package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/ports"
)

// Purchase is an authorization with its latest dispute, if any.
type Purchase struct {
	domain.Authorization
	Dispute *domain.Dispute
}

// Purchases lists a card's purchases, newest first.
func (s *Service) Purchases(ctx context.Context, card uuid.UUID, limit int) ([]Purchase, error) {
	auths, err := s.store.AuthorizationsByCard(ctx, card, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(auths))
	for i, a := range auths {
		ids[i] = a.ID
	}
	disputes, err := s.store.LatestDisputes(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Purchase, len(auths))
	for i, a := range auths {
		out[i] = Purchase{Authorization: a}
		if d, ok := disputes[a.ID]; ok {
			out[i].Dispute = &d
		}
	}
	return out, nil
}

// Refund is a merchant refund sent by the network, idempotent on reference.
// It cannot eat into an amount under an open dispute.
func (s *Service) Refund(ctx context.Context, authorization uuid.UUID, reference string, amount int64) (domain.Refund, error) {
	var r domain.Refund
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		a, err := tx.LockAuthorization(authorization)
		if err != nil {
			return err
		}
		existing, err := tx.RefundByReference(authorization, reference)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Amount != amount {
				return fmt.Errorf("%w: refund %q was for another amount", domain.ErrInvalidArgument, reference)
			}
			r = *existing
			return nil
		}
		var disputed int64
		open, err := tx.OpenDispute(authorization)
		if err != nil {
			return err
		}
		if open != nil {
			disputed = open.Amount
		}
		r, err = s.startRefund(tx, a, amount, domain.RefundMerchant, reference, disputed)
		return err
	})
	if err != nil || r.Status != domain.RefundPending {
		return r, err
	}
	return s.settleRefund(ctx, r)
}

// startRefund records a refund (in the purchase's unit of work); the money
// moves after the commit.
func (s *Service) startRefund(tx ports.Tx, a domain.Authorization, amount int64, source domain.RefundSource, reference string, disputed int64) (domain.Refund, error) {
	r, err := domain.NewRefund(a, amount, source, reference, disputed, s.now())
	if err != nil {
		return r, err
	}
	if err := tx.InsertRefund(r); err != nil {
		return r, err
	}
	return r, tx.SetRefunded(a.ID, a.Refunded+amount)
}

func refundPayload(r domain.Refund) map[string]any {
	return map[string]any{"refund_id": r.ID, "authorization_id": r.AuthorizationID, "account_id": r.AccountID,
		"amount_minor": r.Amount, "merchant_name": r.MerchantName, "source": string(r.Source), "function": string(r.Function),
		"status": string(r.Status), "failure_reason": r.FailureReason}
}

// settleRefund moves the money (idempotent on the refund) and records how it
// ended. A technical error leaves it PENDING for the recovery worker.
func (s *Service) settleRefund(ctx context.Context, r domain.Refund) (domain.Refund, error) {
	funds := s.txs
	if r.Function == domain.Credit {
		funds = s.credit
	}
	amount, err := money.New(r.Amount, money.BRL)
	if err != nil {
		return r, err
	}
	res, err := funds.Refund(ctx, r.Key(), r.AccountID, amount, "Estorno "+r.MerchantName, map[string]string{
		"authorization_id": r.AuthorizationID.String(), "refund_id": r.ID.String(), "counterparty_name": r.MerchantName,
		"refund_source": string(r.Source)})
	if err != nil {
		s.log.WarnContext(ctx, "refund not settled yet, will retry", "refund_id", r.ID, "error", err)
		return r, nil
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		a, err := tx.LockAuthorization(r.AuthorizationID)
		if err != nil {
			return err
		}
		cur, err := tx.LockRefund(r.ID)
		if err != nil {
			return err
		}
		if cur.Status != domain.RefundPending {
			r = cur // settled concurrently
			return nil
		}
		eventType := "card.purchase.refunded"
		if res.Status == "COMPLETED" {
			cur.Status = domain.RefundCompleted
		} else {
			cur.Status, cur.FailureReason, eventType = domain.RefundFailed, res.FailureReason, "card.refund.failed"
			if err := tx.SetRefunded(a.ID, max(a.Refunded-cur.Amount, 0)); err != nil {
				return err
			}
		}
		if err := tx.SaveRefund(cur); err != nil {
			return err
		}
		r = cur
		ev, err := s.event(ctx, eventType, r.ID.String(), refundPayload(r))
		if err != nil {
			return err
		}
		return tx.Publish(ev)
	})
	return r, err
}

func disputePayload(d domain.Dispute) map[string]any {
	return map[string]any{"dispute_id": d.ID, "authorization_id": d.AuthorizationID, "card_id": d.CardID,
		"account_id": d.AccountID, "reason": string(d.Reason), "amount_minor": d.Amount, "merchant_name": d.MerchantName,
		"status": string(d.Status), "decision_reason": d.DecisionReason, "decided_by": d.DecidedBy}
}

// OpenDispute starts a customer's dispute. A purchase the customer does not
// recognize is fraud: the card is blocked in the same unit of work.
// card, when set, is the card the purchase must belong to.
func (s *Service) OpenDispute(ctx context.Context, card, authorization uuid.UUID, reason domain.DisputeReason, amount int64, description string) (domain.Dispute, error) {
	var d domain.Dispute
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		a, err := tx.LockAuthorization(authorization)
		if err != nil {
			return err
		}
		if a.CardID == uuid.Nil || (card != uuid.Nil && a.CardID != card) {
			return fmt.Errorf("%w: authorization", domain.ErrNotFound)
		}
		open, err := tx.OpenDispute(authorization)
		if err != nil {
			return err
		}
		if d, err = domain.NewDispute(a, reason, amount, description, open != nil, s.now()); err != nil {
			return err
		}
		if err := tx.InsertDispute(d); err != nil {
			return err
		}
		if reason == domain.DisputeNotRecognized {
			c, err := tx.LockCard(a.CardID)
			if err != nil {
				return err
			}
			if c.Status == domain.Active {
				if err := c.Block(domain.ReasonFraudSuspected); err != nil {
					return err
				}
				ev, err := s.event(ctx, "card.blocked", c.ID.String(), cardPayload(c))
				if err != nil {
					return err
				}
				if err := errors.Join(tx.SaveCard(c), tx.Publish(ev)); err != nil {
					return err
				}
			}
		}
		ev, err := s.event(ctx, "card.dispute.opened", d.ID.String(), disputePayload(d))
		if err != nil {
			return err
		}
		return tx.Publish(ev)
	})
	return d, err
}

func (s *Service) Dispute(ctx context.Context, id uuid.UUID) (domain.Dispute, error) {
	return s.store.Dispute(ctx, id)
}

func (s *Service) Disputes(ctx context.Context, account uuid.UUID, status domain.DisputeStatus, limit int) ([]domain.Dispute, error) {
	return s.store.Disputes(ctx, account, status, limit)
}

// DecideDispute records an analyst's decision. Won: the disputed amount is
// refunded (recorded with the decision, the money moves right after and is
// retried if needed). Lost: the customer is told why.
func (s *Service) DecideDispute(ctx context.Context, id uuid.UUID, analyst string, won bool, reason string) (domain.Dispute, error) {
	d, err := s.store.Dispute(ctx, id)
	if err != nil {
		return d, err
	}
	var refund *domain.Refund
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		a, err := tx.LockAuthorization(d.AuthorizationID)
		if err != nil {
			return err
		}
		if d, err = tx.LockDispute(id); err != nil {
			return err
		}
		if err := d.Decide(analyst, won, reason, s.now()); err != nil {
			return err
		}
		if err := tx.SaveDispute(d); err != nil {
			return err
		}
		if won {
			r, err := s.startRefund(tx, a, d.Amount, domain.RefundDispute, d.ID.String(), 0)
			if err != nil {
				return err
			}
			refund = &r
		}
		ev, err := s.event(ctx, "card.dispute.decided", d.ID.String(), disputePayload(d))
		if err != nil {
			return err
		}
		return tx.Publish(ev)
	})
	if err != nil {
		return d, err
	}
	if refund != nil {
		if _, err := s.settleRefund(ctx, *refund); err != nil {
			s.log.WarnContext(ctx, "dispute refund will be retried", "dispute_id", d.ID, "error", err)
		}
	}
	return d, nil
}

// recoverRefunds settles refunds left PENDING (an outage after recording them).
func (s *Service) recoverRefunds(ctx context.Context, olderThan time.Duration) {
	list, err := s.store.PendingRefunds(ctx, s.now().Add(-olderThan), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "pending refunds scan", "error", err)
		return
	}
	for _, r := range list {
		if _, err := s.settleRefund(ctx, r); err != nil {
			s.log.WarnContext(ctx, "refund settlement failed", "refund_id", r.ID, "error", err)
		}
	}
}
