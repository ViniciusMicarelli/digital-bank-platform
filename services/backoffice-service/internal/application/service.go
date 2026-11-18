// Package application runs the back-office use cases: recording and
// verifying the audit trail, and four-eyes approvals.
package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/ports"
)

type Service struct {
	store ports.Store
	exec  ports.Executor
	log   *slog.Logger
	now   func() time.Time
}

func New(store ports.Store, exec ports.Executor, log *slog.Logger) *Service {
	return &Service{store: store, exec: exec, log: log, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Record appends a staff action to the audit trail.
func (s *Service) Record(ctx context.Context, e domain.Event) (domain.Event, error) {
	if err := e.Validate(); err != nil {
		return e, err
	}
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		var err error
		e, err = tx.AppendAudit(ctx, e, s.now())
		return err
	})
	return e, err
}

func (s *Service) Audit(ctx context.Context, f ports.AuditFilter) ([]domain.Event, error) {
	return s.store.ListAudit(ctx, f)
}

// Verification is the result of recomputing the chain.
type Verification struct {
	Intact   bool
	Events   int64
	BrokenAt int64
}

// Verify recomputes every hash and checks each event links to the one
// before it: an edited, removed or reordered event is found.
func (s *Service) Verify(ctx context.Context) (Verification, error) {
	v := Verification{Intact: true}
	prev := domain.GenesisHash
	err := s.store.ScanAudit(ctx, func(e domain.Event) error {
		v.Events++
		if v.Intact && (e.Seq != v.Events || e.PrevHash != prev || e.ComputeHash() != e.Hash) {
			v.Intact, v.BrokenAt = false, e.Seq
		}
		prev = e.Hash
		return nil
	})
	return v, err
}

func approvalEvent(actor, action string, a domain.Approval, extra map[string]string) domain.Event {
	d := map[string]string{"approval_id": a.ID.String(), "kind": string(a.Kind)}
	for k, v := range extra {
		d[k] = v
	}
	return domain.Event{ActorID: actor, Action: action, TargetType: "account", TargetID: a.TargetID, Details: d}
}

// Request opens a four-eyes approval; the request is audited with it.
func (s *Service) Request(ctx context.Context, kind domain.Kind, target, reason, requestedBy string) (domain.Approval, error) {
	a, err := domain.NewApproval(kind, target, reason, requestedBy, s.now())
	if err != nil {
		return a, err
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		if err := tx.InsertApproval(ctx, a); err != nil {
			return err
		}
		_, err := tx.AppendAudit(ctx, approvalEvent(requestedBy, "APPROVAL_REQUESTED", a, map[string]string{"reason": a.Reason}), s.now())
		return err
	})
	return a, err
}

func (s *Service) Approval(ctx context.Context, id uuid.UUID) (domain.Approval, error) {
	return s.store.Approval(ctx, id)
}

func (s *Service) Approvals(ctx context.Context, status domain.Status, limit int) ([]domain.Approval, error) {
	return s.store.ListApprovals(ctx, status, limit)
}

// Decide records the second analyst's decision and, when approved, runs
// the action. A transient failure leaves it APPROVED for the recovery loop.
func (s *Service) Decide(ctx context.Context, id uuid.UUID, approver string, approve bool, reason string) (domain.Approval, error) {
	var (
		a         domain.Approval
		decideErr error
	)
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		var err error
		if a, err = tx.ApprovalForUpdate(ctx, id); err != nil {
			return err
		}
		now := s.now()
		if a.Expired(now) {
			// Keep the expiry (and its audit event) even though the decision fails.
			a.Status = domain.StatusExpired
			decideErr = domain.Refuse("APPROVAL_EXPIRED", "this request expired; open a new one")
			if err := tx.SaveApproval(ctx, a); err != nil {
				return err
			}
			_, err := tx.AppendAudit(ctx, approvalEvent("system", "APPROVAL_EXPIRED", a, nil), now)
			return err
		}
		if err := a.Decide(approver, approve, reason, now); err != nil {
			decideErr = err
			return nil
		}
		if err := tx.SaveApproval(ctx, a); err != nil {
			return err
		}
		action, extra := "APPROVAL_APPROVED", map[string]string{}
		if !approve {
			action, extra["reason"] = "APPROVAL_REJECTED", a.DecisionReason
		}
		_, err = tx.AppendAudit(ctx, approvalEvent(approver, action, a, extra), now)
		return err
	})
	if err != nil {
		return a, err
	}
	if decideErr != nil {
		return a, decideErr
	}
	if a.Status == domain.StatusApproved {
		return s.execute(ctx, a)
	}
	return a, nil
}

// execute runs an approved action once and records how it ended.
func (s *Service) execute(ctx context.Context, a domain.Approval) (domain.Approval, error) {
	runErr := s.exec.Execute(ctx, a)
	refusal, permanent := domain.AsRefused(runErr)
	if runErr != nil && !permanent {
		s.log.WarnContext(ctx, "approved action failed, will retry", "approval_id", a.ID, "error", runErr)
		return a, nil
	}
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		cur, err := tx.ApprovalForUpdate(ctx, a.ID)
		if err != nil {
			return err
		}
		if cur.Status != domain.StatusApproved {
			a = cur // settled concurrently (recovery)
			return nil
		}
		action, extra := "APPROVAL_EXECUTED", map[string]string(nil)
		if permanent {
			cur.Settle(refusal.Reason)
			action, extra = "APPROVAL_FAILED", map[string]string{"failure_reason": refusal.Reason}
		} else {
			cur.Settle("")
		}
		if err := tx.SaveApproval(ctx, cur); err != nil {
			return err
		}
		a = cur
		_, err = tx.AppendAudit(ctx, approvalEvent(cur.DecidedBy, action, cur, extra), s.now())
		return err
	})
	return a, err
}

// RecoverOnce expires stale requests and retries approved actions that
// have not settled (a crash or an outage after the approval).
func (s *Service) RecoverOnce(ctx context.Context, settleAfter time.Duration) {
	now := s.now()
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		expired, err := tx.ExpirePending(ctx, now)
		if err != nil {
			return err
		}
		for _, a := range expired {
			if _, err := tx.AppendAudit(ctx, approvalEvent("system", "APPROVAL_EXPIRED", a, nil), now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.log.ErrorContext(ctx, "expire approvals", "error", err)
	}
	stuck, err := s.store.ApprovedBefore(ctx, now.Add(-settleAfter))
	if err != nil {
		s.log.ErrorContext(ctx, "list approved actions", "error", err)
		return
	}
	for _, a := range stuck {
		if _, err := s.execute(ctx, a); err != nil && !errors.Is(err, context.Canceled) {
			s.log.ErrorContext(ctx, "settle approval", "approval_id", a.ID, "error", err)
		}
	}
}

// RunRecovery calls RecoverOnce every interval until ctx ends.
func (s *Service) RunRecovery(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RecoverOnce(ctx, interval)
		}
	}
}
