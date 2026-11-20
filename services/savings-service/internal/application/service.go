// Package application runs savings boxes: opening them, moving money in and
// out through transaction-service, and paying the daily yield.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/ports"
)

// SavingsYieldAccountID is the ledger's internal account that pays yields.
var SavingsYieldAccountID = uuid.MustParse("00000000-0000-0000-0000-000000000007")

type Service struct {
	store    ports.Store
	ledger   ports.Ledger
	txs      ports.Transactions
	log      *slog.Logger
	now      func() time.Time
	annualBP int
}

func New(store ports.Store, ledger ports.Ledger, txs ports.Transactions, log *slog.Logger, annualBP int) *Service {
	return &Service{store: store, ledger: ledger, txs: txs, log: log, now: time.Now, annualBP: annualBP}
}

func (s *Service) SetClock(now func() time.Time) { s.now = now }

// AnnualRateBP is the yearly rate paid on every box.
func (s *Service) AnnualRateBP() int { return s.annualBP }

// View is a box with its balance (read from the ledger).
type View struct {
	domain.Box
	Balance int64
}

func (s *Service) view(ctx context.Context, b domain.Box) (View, error) {
	bal, err := s.ledger.Balance(ctx, b.ID)
	return View{Box: b, Balance: bal}, err
}

// Create opens a box for a checking account (idempotent on key).
func (s *Service) Create(ctx context.Context, key string, account uuid.UUID, name string, goal int64) (View, error) {
	if existing, err := s.store.BoxByKey(ctx, key); err != nil || existing != nil {
		if existing != nil && existing.AccountID != account {
			return View{}, fmt.Errorf("%w: %q", domain.ErrConflict, key)
		}
		if existing == nil {
			return View{}, err
		}
		if err := s.ledger.CreateBoxAccount(ctx, existing.ID); err != nil { // a retry after a failure
			return View{}, err
		}
		return s.view(ctx, *existing)
	}
	b, err := domain.NewBox(key, account, name, goal, s.now())
	if err != nil {
		return View{}, err
	}
	if err := s.ledger.CreateBoxAccount(ctx, b.ID); err != nil {
		return View{}, err
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		n, err := tx.CountActive(account)
		if err != nil {
			return err
		}
		if n >= domain.MaxBoxes {
			return domain.Refuse("TOO_MANY_BOXES", fmt.Sprintf("at most %d boxes per account", domain.MaxBoxes))
		}
		_, err = tx.InsertBox(b)
		return err
	})
	if err != nil {
		return View{}, err
	}
	stored, err := s.store.BoxByKey(ctx, key)
	if err != nil || stored == nil {
		return View{}, errors.Join(err, domain.ErrNotFound)
	}
	return s.view(ctx, *stored)
}

func (s *Service) Box(ctx context.Context, id uuid.UUID) (View, error) {
	b, err := s.store.Box(ctx, id)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, b)
}

func (s *Service) Boxes(ctx context.Context, account uuid.UUID) ([]View, error) {
	boxes, err := s.store.Boxes(ctx, account)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(boxes))
	for _, b := range boxes {
		v, err := s.view(ctx, b)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, name string, goal int64) (View, error) {
	var b domain.Box
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		var err error
		if b, err = tx.LockBox(id); err != nil {
			return err
		}
		if err := b.Update(name, goal); err != nil {
			return err
		}
		return tx.SaveBox(b)
	})
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, b)
}

func (s *Service) Movements(ctx context.Context, box uuid.UUID, limit int) ([]domain.Movement, error) {
	return s.store.Movements(ctx, box, limit)
}

// Deposit moves money from the checking account into the box.
func (s *Service) Deposit(ctx context.Context, key string, box uuid.UUID, amount int64) (domain.Movement, View, error) {
	return s.move(ctx, key, box, domain.Deposit, amount)
}

// Withdraw moves money from the box back to the checking account.
func (s *Service) Withdraw(ctx context.Context, key string, box uuid.UUID, amount int64) (domain.Movement, View, error) {
	return s.move(ctx, key, box, domain.Withdrawal, amount)
}

func (s *Service) move(ctx context.Context, key string, boxID uuid.UUID, kind domain.Kind, amount int64) (domain.Movement, View, error) {
	b, err := s.store.Box(ctx, boxID)
	if err != nil {
		return domain.Movement{}, View{}, err
	}
	m, err := domain.NewMovement(b, key, kind, amount, s.now())
	if err != nil {
		return domain.Movement{}, View{}, err
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		existing, err := tx.InsertMovement(m)
		if existing != nil {
			if existing.Kind != kind || existing.Amount != amount {
				return fmt.Errorf("%w: %q", domain.ErrConflict, key)
			}
			m = *existing
		}
		return err
	})
	if err != nil {
		return domain.Movement{}, View{}, err
	}
	if m, err = s.settle(ctx, m, b); err != nil {
		return m, View{}, err
	}
	v, err := s.Box(ctx, boxID)
	return m, v, err
}

// settle runs a PENDING movement in transaction-service (idempotent on the
// movement) and records the outcome. A technical error leaves it PENDING
// for the recovery loop.
func (s *Service) settle(ctx context.Context, m domain.Movement, b domain.Box) (domain.Movement, error) {
	if m.Status != domain.Pending {
		return m, nil
	}
	typ, from, to, desc := "SAVINGS_DEPOSIT", b.AccountID, b.ID, "Guardado em "+b.Name
	switch m.Kind {
	case domain.Withdrawal:
		typ, from, to, desc = "SAVINGS_WITHDRAWAL", b.ID, b.AccountID, "Resgate de "+b.Name
	case domain.Yield:
		typ, from, to, desc = "SAVINGS_YIELD", SavingsYieldAccountID, b.ID, "Rendimento de "+b.Name
	}
	res, err := s.txs.Post(ctx, m.TransferKey(), typ, from, to, m.Amount, desc, map[string]string{
		"box_id": b.ID.String(), "movement_id": m.ID.String(), "counterparty_name": b.Name})
	if err != nil {
		s.log.WarnContext(ctx, "savings movement not settled yet, will retry", "movement_id", m.ID, "error", err)
		return m, nil
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		box, err := tx.LockBox(b.ID)
		if err != nil {
			return err
		}
		cur, err := tx.LockMovement(m.ID)
		if err != nil {
			return err
		}
		if cur.Status != domain.Pending {
			m = cur
			return nil
		}
		if res.Status == "COMPLETED" {
			cur.Status = domain.Completed
			if cur.Kind == domain.Yield {
				box.Yielded += cur.Amount
				if err := tx.SaveBox(box); err != nil {
					return err
				}
			}
		} else {
			cur.Status, cur.FailureReason = domain.Failed, res.FailureReason
		}
		m = cur
		return tx.SaveMovement(cur)
	})
	return m, err
}

// Close moves everything back to the checking account and closes the box.
func (s *Service) Close(ctx context.Context, key string, id uuid.UUID) (View, error) {
	v, err := s.Box(ctx, id)
	if err != nil {
		return View{}, err
	}
	if v.Status == domain.Closed {
		return v, nil
	}
	if v.Balance > 0 {
		m, _, err := s.Withdraw(ctx, "close:"+key, id, v.Balance)
		if err != nil {
			return View{}, err
		}
		if m.Status != domain.Completed {
			return View{}, domain.Refuse("CLOSE_FAILED", "the money could not be moved back yet; try again")
		}
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		b, err := tx.LockBox(id)
		if err != nil {
			return err
		}
		b.Status, b.Schedule = domain.Closed, domain.Schedule{}
		return tx.SaveBox(b)
	})
	if err != nil {
		return View{}, err
	}
	return s.Box(ctx, id)
}

// AccrueOnce pays the yield of every day that ended and was not paid yet,
// on the balance at the end of that day.
func (s *Service) AccrueOnce(ctx context.Context) {
	today := domain.Day(s.now())
	ids, err := s.store.BoxesToAccrue(ctx, today.AddDate(0, 0, -1), 200)
	if err != nil {
		s.log.ErrorContext(ctx, "boxes to accrue", "error", err)
		return
	}
	for _, id := range ids {
		if err := s.accrueBox(ctx, id, today); err != nil {
			s.log.WarnContext(ctx, "yield not paid yet, will retry", "box_id", id, "error", err)
		}
	}
}

func (s *Service) accrueBox(ctx context.Context, id uuid.UUID, today time.Time) error {
	for {
		b, err := s.store.Box(ctx, id)
		if err != nil {
			return err
		}
		day := b.LastAccrued.AddDate(0, 0, 1)
		if !day.Before(today) || b.Status != domain.Active {
			return nil
		}
		balance, err := s.ledger.BalanceAt(ctx, b.ID, domain.EndOfDay(day))
		if err != nil {
			return err
		}
		amount, carry := domain.Accrue(balance, s.annualBP, 1, b.YieldCarryMicro)
		if amount > 0 {
			m := domain.Movement{ID: uuid.Must(uuid.NewV7()), BoxID: b.ID, IdempotencyKey: "yield:" + day.Format(time.DateOnly),
				Kind: domain.Yield, Amount: amount, Status: domain.Pending, CreatedAt: domain.EndOfDay(day)}
			err := s.store.InTx(ctx, func(tx ports.Tx) error {
				existing, err := tx.InsertMovement(m)
				if existing != nil {
					m = *existing
				}
				return err
			})
			if err != nil {
				return err
			}
			if m, err = s.settle(ctx, m, b); err != nil {
				return err
			}
			if m.Status == domain.Pending {
				return errors.New("yield posting pending")
			}
		}
		err = s.store.InTx(ctx, func(tx ports.Tx) error {
			cur, err := tx.LockBox(id)
			if err != nil || !cur.LastAccrued.Before(day) {
				return err
			}
			cur.LastAccrued, cur.YieldCarryMicro = day, carry
			return tx.SaveBox(cur)
		})
		if err != nil {
			return err
		}
	}
}

// SimulateYield (sandbox) pays days of yield at once on today's balance.
func (s *Service) SimulateYield(ctx context.Context, id uuid.UUID, days int) (domain.Movement, View, error) {
	if days < 1 || days > 3650 {
		return domain.Movement{}, View{}, fmt.Errorf("%w: days must be between 1 and 3650", domain.ErrInvalidArgument)
	}
	v, err := s.Box(ctx, id)
	if err != nil {
		return domain.Movement{}, View{}, err
	}
	if v.Status != domain.Active {
		return domain.Movement{}, View{}, domain.Refuse("BOX_CLOSED", "this box is closed")
	}
	amount, _ := domain.Accrue(v.Balance, s.annualBP, days, 0)
	if amount == 0 {
		return domain.Movement{}, v, domain.Refuse("NOTHING_TO_YIELD", "put some money in the box first")
	}
	m := domain.Movement{ID: uuid.Must(uuid.NewV7()), BoxID: id, IdempotencyKey: "simulated:" + uuid.NewString(),
		Kind: domain.Yield, Amount: amount, Status: domain.Pending, CreatedAt: s.now()}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		_, err := tx.InsertMovement(m)
		return err
	})
	if err != nil {
		return domain.Movement{}, View{}, err
	}
	if m, err = s.settle(ctx, m, v.Box); err != nil {
		return m, View{}, err
	}
	v, err = s.Box(ctx, id)
	return m, v, err
}

// RecoverOnce settles movements left PENDING by an outage.
func (s *Service) RecoverOnce(ctx context.Context, olderThan time.Duration) {
	list, err := s.store.PendingMovements(ctx, s.now().Add(-olderThan), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "pending movements", "error", err)
		return
	}
	for _, m := range list {
		b, err := s.store.Box(ctx, m.BoxID)
		if err == nil {
			_, err = s.settle(ctx, m, b)
		}
		if err != nil {
			s.log.WarnContext(ctx, "settle movement", "movement_id", m.ID, "error", err)
		}
	}
}

// Run pays yields and settles pending movements every interval.
func (s *Service) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.RecoverOnce(ctx, interval)
		s.AccrueOnce(ctx)
		s.ScheduleOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// SetSchedule sets (or, with amount 0, removes) the box's monthly deposit.
func (s *Service) SetSchedule(ctx context.Context, id uuid.UUID, amount int64, day int) (View, error) {
	var b domain.Box
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		var err error
		if b, err = tx.LockBox(id); err != nil {
			return err
		}
		if err := b.SetSchedule(amount, day, s.now()); err != nil {
			return err
		}
		return tx.SaveBox(b)
	})
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, b)
}

// ScheduleOnce runs every monthly deposit that is due.
func (s *Service) ScheduleOnce(ctx context.Context) {
	ids, err := s.store.BoxesScheduled(ctx, domain.Day(s.now()), 200)
	if err != nil {
		s.log.ErrorContext(ctx, "scheduled deposits", "error", err)
		return
	}
	for _, id := range ids {
		if _, err := s.runSchedule(ctx, id, false); err != nil {
			s.log.WarnContext(ctx, "scheduled deposit not run yet, will retry", "box_id", id, "error", err)
		}
	}
}

// runSchedule makes the deposit of the box's next run and moves the
// schedule on to the first run after today (missed months are not caught
// up). With early, the next run is brought forward (sandbox). A deposit
// refused for lack of funds is skipped, not retried, and its reason kept on
// the box; a technical failure keeps the run due for the next pass
// (idempotent on the run's day).
func (s *Service) runSchedule(ctx context.Context, id uuid.UUID, early bool) (domain.Movement, error) {
	b, err := s.store.Box(ctx, id)
	if err != nil {
		return domain.Movement{}, err
	}
	if !b.Schedule.Active() || b.Status != domain.Active {
		return domain.Movement{}, domain.Refuse("NO_SCHEDULE", "this box has no monthly deposit")
	}
	today, day := domain.Day(s.now()), b.Schedule.Next
	if day.After(today) && !early {
		return domain.Movement{}, nil
	}
	m := domain.Movement{ID: uuid.Must(uuid.NewV7()), BoxID: b.ID, IdempotencyKey: domain.ScheduleKey(day),
		Kind: domain.Deposit, Amount: b.Schedule.Amount, Status: domain.Pending, CreatedAt: s.now()}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		existing, err := tx.InsertMovement(m)
		if existing != nil {
			m = *existing
		}
		return err
	})
	if err != nil {
		return m, err
	}
	if m, err = s.settle(ctx, m, b); err != nil {
		return m, err
	}
	if m.Status == domain.Pending {
		return m, errors.New("scheduled deposit pending")
	}
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		cur, err := tx.LockBox(id)
		if err != nil || !cur.Schedule.Active() || !cur.Schedule.Next.Equal(day) {
			return err // removed or changed meanwhile, or already moved on
		}
		cur.Schedule.Next = domain.NextRun(cur.Schedule.Day, maxDay(day, today))
		cur.Schedule.LastFailure = m.FailureReason
		return tx.SaveBox(cur)
	})
	return m, err
}

func maxDay(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// RunSchedule (sandbox) makes the monthly deposit now, as if it were due
// today; the next run moves to the following month.
func (s *Service) RunSchedule(ctx context.Context, id uuid.UUID) (domain.Movement, View, error) {
	m, err := s.runSchedule(ctx, id, true)
	if err != nil {
		return m, View{}, err
	}
	v, err := s.Box(ctx, id)
	return m, v, err
}
