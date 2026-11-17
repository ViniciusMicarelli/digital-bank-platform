package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/ports"
)

// Schedule creates a future (optionally recurring) Pix. The key is resolved
// now so typos fail at once, and again at each execution.
func (s *Service) Schedule(ctx context.Context, idemKey string, account uuid.UUID, key string, amount money.Money,
	description string, start time.Time, freq domain.Frequency, occurrences int) (domain.Schedule, error) {
	holder, err := s.accounts.Holder(ctx, account)
	if err != nil {
		return domain.Schedule{}, err
	}
	if !holder.Active {
		return domain.Schedule{}, ErrAccountNotActive
	}
	_, value, _, receiverAccount, err := s.Lookup(ctx, key)
	if err != nil {
		return domain.Schedule{}, err
	}
	if receiverAccount == account {
		return domain.Schedule{}, fmt.Errorf("%w: cannot schedule a Pix to the same account", domain.ErrInvalidArgument)
	}
	sc, err := domain.NewSchedule(idemKey, account, value, amount, description, start, freq, occurrences, domain.Today(s.now()))
	if err != nil {
		return domain.Schedule{}, err
	}
	existing, err := s.store.InsertSchedule(ctx, sc)
	if err != nil {
		return domain.Schedule{}, err
	}
	if existing != nil {
		if existing.AccountID != account || existing.Amount != amount || existing.Key != value {
			return domain.Schedule{}, fmt.Errorf("%w: %q", domain.ErrIdempotencyConflict, idemKey)
		}
		return *existing, nil
	}
	return sc, nil
}

func (s *Service) Schedules(ctx context.Context, account uuid.UUID) ([]domain.Schedule, error) {
	return s.store.Schedules(ctx, account)
}

func (s *Service) ScheduleRuns(ctx context.Context, account, id uuid.UUID) (domain.Schedule, []ports.ScheduleRun, error) {
	sc, err := s.store.Schedule(ctx, id)
	if err != nil || sc.AccountID != account {
		return domain.Schedule{}, nil, fmt.Errorf("%w: schedule", domain.ErrNotFound)
	}
	runs, err := s.store.ScheduleRuns(ctx, id)
	return sc, runs, err
}

func (s *Service) CancelSchedule(ctx context.Context, account, id uuid.UUID) (domain.Schedule, error) {
	sc, err := s.store.Schedule(ctx, id)
	if err != nil || sc.AccountID != account {
		return domain.Schedule{}, fmt.Errorf("%w: schedule", domain.ErrNotFound)
	}
	if err := sc.Cancel(); err != nil {
		return domain.Schedule{}, err
	}
	return sc, s.store.SaveScheduleStatus(ctx, sc)
}

// RunDueSchedules executes every schedule due on asOf (São Paulo date).
// Each occurrence has its own transfer idempotency key, so a retried run
// never pays twice. A refused payment (no funds, unknown key) is recorded
// and the schedule moves on, like a bank does; technical errors are retried
// on the next tick.
func (s *Service) RunDueSchedules(ctx context.Context, asOf time.Time) (int, error) {
	today := domain.Today(asOf)
	return s.store.ProcessDueSchedules(ctx, today, 500, func(sc *domain.Schedule) (ports.ScheduleRun, error) {
		n := sc.Executed + 1
		run := ports.ScheduleRun{Occurrence: n, Date: sc.NextDate}
		if sc.MandateID != uuid.Nil {
			m, err := s.store.Mandate(ctx, sc.MandateID)
			if err != nil {
				return run, err
			}
			if m.Status != domain.MandateActive {
				sc.Status = domain.ScheduleCancelled
				run.Status, run.Reason = "CANCELLED", "MANDATE_"+string(m.Status)
				return run, nil
			}
		}
		t, err := s.Send(ctx, sc.TransferKey(n), sc.AccountID, sc.Key, sc.Amount, sc.Description)
		switch {
		case err == nil:
			run.TransferID, run.Status, run.Reason = t.ID, string(t.Status), t.FailureReason
		case isBusinessError(err):
			run.Status, run.Reason = "FAILED", err.Error()
		default:
			return run, err
		}
		sc.Advance()
		return run, nil
	})
}

func isBusinessError(err error) bool {
	return errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInvalidArgument) ||
		errors.Is(err, ErrAccountNotActive) || errors.Is(err, domain.ErrIdempotencyConflict)
}

// RunSchedules executes due schedules every interval.
func (s *Service) RunSchedules(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := s.RunDueSchedules(ctx, s.now()); err != nil && ctx.Err() == nil {
			s.log.WarnContext(ctx, "scheduled pix", "error", err)
		} else if n > 0 {
			s.log.InfoContext(ctx, "scheduled pix executed", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// --- Pix Automático ---

// RequestMandate is the receiver asking a payer (identified by one of their
// Pix keys at this bank) for a recurring authorization.
func (s *Service) RequestMandate(ctx context.Context, receiver uuid.UUID, receiverKey, payerKey string, max money.Money, freq domain.Frequency, description string) (domain.Mandate, error) {
	holder, err := s.accounts.Holder(ctx, receiver)
	if err != nil {
		return domain.Mandate{}, err
	}
	_, rKey, err := domain.DetectKeyType(receiverKey)
	if err != nil {
		return domain.Mandate{}, err
	}
	if k, err := s.store.KeyByValue(ctx, rKey); err != nil || k.AccountID != receiver {
		return domain.Mandate{}, fmt.Errorf("%w: the receiver key must belong to the receiver's account", domain.ErrInvalidArgument)
	}
	_, pKey, err := domain.DetectKeyType(payerKey)
	if err != nil {
		return domain.Mandate{}, err
	}
	payer, err := s.store.KeyByValue(ctx, pKey)
	if err != nil {
		return domain.Mandate{}, fmt.Errorf("%w: payer key (Pix Automático between customers of this bank)", domain.ErrNotFound)
	}
	m, err := domain.NewMandate(receiver, rKey, holder.Name, payer.AccountID, max, freq, description)
	if err != nil {
		return domain.Mandate{}, err
	}
	m.CreatedAt = s.now()
	return m, s.store.InsertMandate(ctx, m)
}

func (s *Service) Mandates(ctx context.Context, account uuid.UUID) ([]domain.Mandate, error) {
	return s.store.MandatesByAccount(ctx, account)
}

func (s *Service) DecideMandate(ctx context.Context, payer, id uuid.UUID, approve bool) (domain.Mandate, error) {
	return s.store.UpdateMandate(ctx, id, func(m *domain.Mandate) error { return m.Decide(payer, approve) })
}

func (s *Service) RevokeMandate(ctx context.Context, by, id uuid.UUID) (domain.Mandate, error) {
	return s.store.UpdateMandate(ctx, id, func(m *domain.Mandate) error { return m.Revoke(by) })
}

// ChargeMandate is the receiver charging an active mandate; it becomes a
// Pix scheduled on the due date from the payer to the receiver.
func (s *Service) ChargeMandate(ctx context.Context, idemKey string, receiver, id uuid.UUID, amount money.Money, due time.Time) (domain.Schedule, error) {
	if idemKey == "" {
		return domain.Schedule{}, fmt.Errorf("%w: idempotency key is required", domain.ErrInvalidArgument)
	}
	today := domain.Today(s.now())
	due = time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, time.UTC)
	key := "mandate:" + id.String() + ":" + idemKey
	return s.store.ChargeMandate(ctx, id, key, func(m *domain.Mandate) (domain.Schedule, error) {
		if err := m.Charge(receiver, amount, due, today); err != nil {
			return domain.Schedule{}, err
		}
		sc, err := domain.NewSchedule(key, m.PayerAccount, m.ReceiverKey, amount, m.Description, due, domain.Once, 1, today)
		if err != nil {
			return domain.Schedule{}, err
		}
		sc.MandateID = m.ID
		return sc, nil
	})
}
