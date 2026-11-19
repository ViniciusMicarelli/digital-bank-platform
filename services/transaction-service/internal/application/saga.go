// Package application runs money movements as orchestrated sagas (ADR-0008).
//
// Each step calls a downstream service with an idempotency key derived from
// the transaction id, then persists the new state. Re-running a saga from any
// state is therefore safe, which is what makes recovery simple:
//
//   - business refusals (*domain.RejectedError) compensate and end FAILED;
//   - technical errors leave the saga where it is, and the caller or the
//     recovery worker runs it again later (forward recovery).
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/ports"
)

const (
	source = "transaction-service"
	topic  = "bank.transaction.events"
	// lease bounds how long one run may own a saga before others may take over.
	lease = 30 * time.Second
)

type Service struct {
	store    ports.Store
	ledger   ports.Ledger
	accounts ports.Accounts
	log      *slog.Logger
}

func New(store ports.Store, ledger ports.Ledger, accounts ports.Accounts, log *slog.Logger) *Service {
	return &Service{store: store, ledger: ledger, accounts: accounts, log: log}
}

// Transfer moves money between two customer accounts in one step.
func (s *Service) Transfer(ctx context.Context, key string, from, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (domain.Transaction, error) {
	return s.start(ctx, key, domain.TypeInternalTransfer, from, to, amount, description, metadata)
}

// Post moves money in one step for the credit card types.
func (s *Service) Post(ctx context.Context, key string, typ domain.Type, from, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (domain.Transaction, error) {
	switch typ {
	case domain.TypeInternalTransfer, domain.TypeCreditPurchase, domain.TypeCreditInvoicePayment, domain.TypeCreditCharges, domain.TypeCreditCardRefund:
	default:
		return domain.Transaction{}, fmt.Errorf("%w: %s is not a one-step type", domain.ErrInvalidArgument, typ)
	}
	return s.start(ctx, key, typ, from, to, amount, description, metadata)
}

// ReserveOutgoing consumes the limit and holds the funds; the transaction
// stays RESERVED until Complete or Compensate.
func (s *Service) ReserveOutgoing(ctx context.Context, key string, typ domain.Type, from, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (domain.Transaction, error) {
	if typ != domain.TypePixOut && typ != domain.TypeDebitCard && typ != domain.TypeBoletoPayment {
		return domain.Transaction{}, fmt.Errorf("%w: %s is not a two-phase type", domain.ErrInvalidArgument, typ)
	}
	return s.start(ctx, key, typ, from, to, amount, description, metadata)
}

// CreditIncoming credits money arriving from outside to a customer account.
func (s *Service) CreditIncoming(ctx context.Context, key string, typ domain.Type, from, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (domain.Transaction, error) {
	if typ != domain.TypePixIn && typ != domain.TypeCardRefund {
		return domain.Transaction{}, fmt.Errorf("%w: %s is not an incoming type", domain.ErrInvalidArgument, typ)
	}
	return s.start(ctx, key, typ, from, to, amount, description, metadata)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.Transaction, error) {
	return s.store.Get(ctx, id)
}

func (s *Service) start(ctx context.Context, key string, typ domain.Type, from, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (domain.Transaction, error) {
	t, err := domain.New(key, typ, from, to, amount, description, metadata)
	if err != nil {
		return domain.Transaction{}, err
	}
	existing, fp, err := s.store.Create(ctx, t)
	if err != nil {
		return domain.Transaction{}, err
	}
	if existing != nil {
		if string(fp) != string(t.Fingerprint()) {
			return domain.Transaction{}, fmt.Errorf("%w: %q", domain.ErrIdempotencyConflict, key)
		}
		if existing.Status.Terminal() || existing.Status == domain.StatusReserved {
			return *existing, nil
		}
		t = *existing // unfinished: resume it
	}
	return s.Resume(ctx, t.ID)
}

// Complete captures the hold of a RESERVED transaction. amount (optional)
// captures less than was reserved; the rest goes back to the customer.
func (s *Service) Complete(ctx context.Context, id uuid.UUID, amount *money.Money) (domain.Transaction, error) {
	return s.withLease(ctx, id, func(t *domain.Transaction) error {
		switch t.Status {
		case domain.StatusCompleted:
			return nil
		case domain.StatusReserved:
		default:
			return fmt.Errorf("%w: cannot complete a %s transaction", domain.ErrInvalidState, t.Status)
		}
		if amount != nil {
			if amount.Currency() != t.Amount.Currency() || !amount.IsPositive() || amount.Minor() > t.Amount.Minor() {
				return fmt.Errorf("%w: capture must be positive and at most %s", domain.ErrInvalidArgument, t.Amount.Decimal())
			}
		}
		entryID, err := s.ledger.Capture(ctx, t.HoldID, amount)
		if err != nil {
			return s.stepFailed(ctx, t, "capture", err)
		}
		if amount != nil {
			t.Amount = *amount // what was actually charged
		}
		t.EntryID, t.Status = entryID, domain.StatusCompleted
		return s.save(ctx, *t, ports.Step{Name: "capture", Outcome: "OK"}, "transaction.completed")
	})
}

// Compensate cancels a RESERVED transaction: releases the hold and restores
// the limit. PENDING sagas are never compensated from outside: a step may have
// succeeded downstream with its response lost, so they only move forward
// (replaying idempotent steps returns the original result).
func (s *Service) Compensate(ctx context.Context, id uuid.UUID, reason string) (domain.Transaction, error) {
	return s.withLease(ctx, id, func(t *domain.Transaction) error {
		switch t.Status {
		case domain.StatusFailed:
			return nil
		case domain.StatusReserved, domain.StatusCompensating:
		default:
			return fmt.Errorf("%w: cannot compensate a %s transaction", domain.ErrInvalidState, t.Status)
		}
		if t.Status == domain.StatusReserved {
			if reason == "" {
				reason = "CANCELLED"
			}
			if err := t.StartCompensation(reason); err != nil {
				return err
			}
			if err := s.save(ctx, *t, ports.Step{Name: "compensate", Outcome: "OK", Detail: reason}, ""); err != nil {
				return err
			}
		}
		return s.run(ctx, t)
	})
}

// Resume runs a saga from its current state until it finishes or waits.
func (s *Service) Resume(ctx context.Context, id uuid.UUID) (domain.Transaction, error) {
	return s.withLease(ctx, id, func(t *domain.Transaction) error { return s.run(ctx, t) })
}

// withLease runs fn while owning the saga, always gives the lease back, and
// returns the latest stored state.
func (s *Service) withLease(ctx context.Context, id uuid.UUID, fn func(*domain.Transaction) error) (domain.Transaction, error) {
	t, ok, err := s.store.Acquire(ctx, id, lease)
	if err != nil {
		return domain.Transaction{}, err
	}
	if !ok {
		return t, fmt.Errorf("%w: %s", domain.ErrBusy, id)
	}
	runErr := fn(&t)
	bg := context.WithoutCancel(ctx)
	if err := s.store.Release(bg, id); err != nil {
		s.log.ErrorContext(ctx, "release lease", "transaction_id", id, "error", err)
	}
	latest, err := s.store.Get(bg, id)
	if err != nil {
		return t, errors.Join(runErr, err)
	}
	return latest, runErr
}

// run advances the state machine. It returns nil when the saga is finished
// or waiting (RESERVED), and an error when a technical failure interrupted it.
func (s *Service) run(ctx context.Context, t *domain.Transaction) error {
	for {
		switch t.Status {
		case domain.StatusCompleted, domain.StatusFailed, domain.StatusReserved:
			return nil

		case domain.StatusPending:
			if err := s.forward(ctx, t); err != nil {
				return err
			}

		case domain.StatusCompensating:
			if err := s.compensate(ctx, t); err != nil {
				return err
			}

		default:
			return fmt.Errorf("%w: unknown status %s", domain.ErrInvalidState, t.Status)
		}
	}
}

// forward runs the next step of a PENDING saga.
func (s *Service) forward(ctx context.Context, t *domain.Transaction) error {
	// 1. The receiving customer account must be able to receive money.
	if t.CreditsCustomer() && !t.LimitConsumed && t.HoldID == uuid.Nil && t.EntryID == uuid.Nil {
		if err := s.accounts.EnsureActive(ctx, t.To); err != nil {
			return s.stepFailed(ctx, t, "check_destination", err)
		}
	}

	if t.DebitsCustomerWithoutLimit() && t.EntryID == uuid.Nil {
		if err := s.accounts.EnsureActive(ctx, t.From); err != nil {
			return s.stepFailed(ctx, t, "check_source", err)
		}
	}

	// 2. Limits of the debited account (also rejects inactive accounts).
	if t.ConsumesLimit() && !t.LimitConsumed {
		if err := s.accounts.ConsumeLimit(ctx, t.From, t.ID.String(), t.Amount); err != nil {
			return s.stepFailed(ctx, t, "consume_limit", err)
		}
		t.LimitConsumed = true
		return s.save(ctx, *t, ports.Step{Name: "consume_limit", Outcome: "OK"}, "")
	}

	// 3a. Two-phase: hold the funds and wait for the external confirmation.
	if t.TwoPhase() {
		holdID, err := s.ledger.Hold(ctx, t.HoldKey(), t.From, t.To, t.Amount, t.Description)
		if err != nil {
			return s.stepFailed(ctx, t, "hold", err)
		}
		t.HoldID, t.Status = holdID, domain.StatusReserved
		return s.save(ctx, *t, ports.Step{Name: "hold", Outcome: "OK"}, "transaction.reserved")
	}

	// 3b. One-step: post the entry.
	entryID, err := s.ledger.Transfer(ctx, t.EntryKey(), t.From, t.To, t.Amount, t.Description, s.entryMetadata(t))
	if err != nil {
		return s.stepFailed(ctx, t, "post_entry", err)
	}
	t.EntryID, t.Status = entryID, domain.StatusCompleted
	return s.save(ctx, *t, ports.Step{Name: "post_entry", Outcome: "OK"}, "transaction.completed")
}

// compensate undoes what was done, in reverse order, then fails the saga.
func (s *Service) compensate(ctx context.Context, t *domain.Transaction) error {
	if t.HoldID != uuid.Nil {
		if err := s.ledger.Release(ctx, t.HoldID); err != nil {
			s.recordError(ctx, *t, "release_hold", err)
			return err // keep compensating later; never give up on giving money back
		}
		t.HoldID = uuid.Nil
		if err := s.save(ctx, *t, ports.Step{Name: "release_hold", Outcome: "OK"}, ""); err != nil {
			return err
		}
	}
	if t.LimitConsumed {
		if err := s.accounts.RestoreLimit(ctx, t.From, t.ID.String()); err != nil {
			s.recordError(ctx, *t, "restore_limit", err)
			return err
		}
		t.LimitConsumed = false
		if err := s.save(ctx, *t, ports.Step{Name: "restore_limit", Outcome: "OK"}, ""); err != nil {
			return err
		}
	}
	t.Status = domain.StatusFailed
	return s.save(ctx, *t, ports.Step{Name: "fail", Outcome: "OK", Detail: t.FailureReason}, "transaction.failed")
}

// stepFailed classifies a step error: a business refusal starts compensation
// (and returns nil so run continues); a technical error is returned as-is.
func (s *Service) stepFailed(ctx context.Context, t *domain.Transaction, step string, err error) error {
	rej, ok := domain.AsRejected(err)
	if !ok {
		s.recordError(ctx, *t, step, err)
		return err
	}
	if t.Status == domain.StatusReserved {
		// A refused capture leaves money held; surface it instead of guessing.
		s.recordError(ctx, *t, step, err)
		return fmt.Errorf("%w: %s refused: %v", domain.ErrInvalidState, step, rej)
	}
	if err := t.StartCompensation(rej.Reason); err != nil {
		return err
	}
	return s.save(ctx, *t, ports.Step{Name: step, Outcome: "REJECTED", Detail: rej.Error()}, "")
}

func (s *Service) recordError(ctx context.Context, t domain.Transaction, step string, err error) {
	s.log.WarnContext(ctx, "saga step failed, will retry", "transaction_id", t.ID, "step", step, "error", err)
}

func (s *Service) entryMetadata(t *domain.Transaction) map[string]string {
	md := map[string]string{"transaction_id": t.ID.String(), "type": string(t.Type)}
	for k, v := range t.Metadata {
		md[k] = v
	}
	return md
}

// save persists t with an optional step record and an optional event.
func (s *Service) save(ctx context.Context, t domain.Transaction, step ports.Step, eventType string) error {
	var events []outbox.Message
	if eventType != "" {
		ev, err := outbox.NewEvent(ctx, source, eventType, payload(t))
		if err != nil {
			return err
		}
		events = append(events, outbox.Message{Topic: topic, Key: t.ID.String(), Event: ev})
	}
	if err := s.store.Save(ctx, t, step, events...); err != nil {
		return err
	}
	if eventType != "" {
		telemetry.BusinessEvents.WithLabelValues(eventType, string(t.Type)).Inc()
	}
	return nil
}

// ReservationTimeouts says how long each two-phase type may stay RESERVED
// before it is compensated: a Pix waits minutes for SPI, a card
// authorization days for clearing.
type ReservationTimeouts map[domain.Type]time.Duration

// RecoverOnce resumes stale sagas and compensates reservations that waited
// too long for their external confirmation. It returns how many it touched.
func (s *Service) RecoverOnce(ctx context.Context, staleAfter time.Duration, timeouts ReservationTimeouts) (int, error) {
	now := time.Now()
	shortest := time.Duration(0)
	for _, d := range timeouts {
		if shortest == 0 || d < shortest {
			shortest = d
		}
	}
	candidates, err := s.store.Unfinished(ctx, now.Add(-staleAfter), now.Add(-shortest), 100)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range candidates {
		if c.Status == domain.StatusReserved {
			timeout, ok := timeouts[c.Type]
			if !ok || now.Sub(c.UpdatedAt) < timeout {
				continue
			}
			_, err = s.Compensate(ctx, c.ID, "RESERVATION_TIMEOUT")
		} else {
			_, err = s.Resume(ctx, c.ID)
		}
		n++
		if err != nil && !errors.Is(err, domain.ErrBusy) {
			s.log.WarnContext(ctx, "recovery attempt failed", "transaction_id", c.ID, "error", err)
		}
	}
	return n, nil
}

// RunRecovery calls RecoverOnce every interval until ctx is cancelled.
func (s *Service) RunRecovery(ctx context.Context, interval, staleAfter time.Duration, timeouts ReservationTimeouts) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := s.RecoverOnce(ctx, staleAfter, timeouts); err != nil {
				s.log.ErrorContext(ctx, "recovery scan", "error", err)
			} else if n > 0 {
				s.log.InfoContext(ctx, "recovery scan", "resumed", n)
			}
		}
	}
}

// TransactionPayload is the body of bank.transaction.events.
type TransactionPayload struct {
	TransactionID string            `json:"transaction_id"`
	Type          string            `json:"type"`
	Status        string            `json:"status"`
	FromAccountID string            `json:"from_account_id"`
	ToAccountID   string            `json:"to_account_id"`
	AmountMinor   int64             `json:"amount_minor"`
	Currency      string            `json:"currency"`
	Description   string            `json:"description"`
	Metadata      map[string]string `json:"metadata"`
	FailureReason string            `json:"failure_reason,omitempty"`
	EntryID       string            `json:"entry_id,omitempty"`
	OccurredAt    time.Time         `json:"occurred_at"`
}

func payload(t domain.Transaction) TransactionPayload {
	p := TransactionPayload{
		TransactionID: t.ID.String(), Type: string(t.Type), Status: string(t.Status),
		FromAccountID: t.From.String(), ToAccountID: t.To.String(),
		AmountMinor: t.Amount.Minor(), Currency: string(t.Amount.Currency()),
		Description: t.Description, Metadata: t.Metadata, FailureReason: t.FailureReason,
		OccurredAt: time.Now().UTC(),
	}
	if t.EntryID != uuid.Nil {
		p.EntryID = t.EntryID.String()
	}
	return p
}
