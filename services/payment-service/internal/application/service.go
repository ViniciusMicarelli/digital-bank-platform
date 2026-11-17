// Package application pays boletos: it quotes them against the registry and
// runs each payment as a small state machine over a two-phase saga
// (transaction-service holds the money, the clearing network settles, then
// the hold is captured or released).
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/ports"
)

const (
	source = "payment-service"
	topic  = "bank.payment.events"
	lease  = 30 * time.Second
)

var saoPaulo = func() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*3600)
	}
	return loc
}()

type Service struct {
	store    ports.Store
	registry ports.Registry
	clearing ports.Clearing
	txs      ports.Transactions
	log      *slog.Logger
	now      func() time.Time
}

func New(store ports.Store, registry ports.Registry, clearing ports.Clearing, txs ports.Transactions, log *slog.Logger) *Service {
	return &Service{store: store, registry: registry, clearing: clearing, txs: txs, log: log, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Today is the civil date in São Paulo, as midnight UTC.
func (s *Service) Today() time.Time {
	y, m, d := s.now().In(saoPaulo).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Lookup quotes paying a boleto today.
func (s *Service) Lookup(ctx context.Context, line string) (domain.Quote, error) {
	b, err := domain.ParseBoleto(line, s.Today())
	if err != nil {
		return domain.Quote{}, err
	}
	return s.quote(ctx, b, b.Amount, s.Today())
}

func (s *Service) quote(ctx context.Context, b domain.Boleto, nominal int64, on time.Time) (domain.Quote, error) {
	payee, err := s.registry.Lookup(ctx, b)
	if err != nil {
		return domain.Quote{}, err
	}
	q := domain.Quote{Boleto: b, Payee: payee, Nominal: nominal}
	if nominal > 0 {
		if q.Fine, q.Interest, err = domain.Charges(nominal, b.DueDate, on); err != nil {
			return domain.Quote{}, err
		}
	}
	return q, nil
}

// Pay pays a boleto from account, now or on scheduledFor (zero: today).
func (s *Service) Pay(ctx context.Context, key string, account uuid.UUID, line string, amount int64, scheduledFor time.Time) (domain.Payment, error) {
	today := s.Today()
	b, err := domain.ParseBoleto(line, today)
	if err != nil {
		return domain.Payment{}, err
	}
	switch {
	case key == "":
		return domain.Payment{}, fmt.Errorf("%w: idempotency key is required", domain.ErrInvalidArgument)
	case b.Amount == 0 && amount <= 0:
		return domain.Payment{}, fmt.Errorf("%w: this boleto has no amount; inform it", domain.ErrInvalidArgument)
	case b.Amount > 0 && amount > 0 && amount != b.Amount:
		return domain.Payment{}, fmt.Errorf("%w: this boleto's amount is fixed", domain.ErrInvalidArgument)
	}
	nominal := b.Amount
	if nominal == 0 {
		nominal = amount
	}
	date := today
	if !scheduledFor.IsZero() {
		switch {
		case scheduledFor.Before(today):
			return domain.Payment{}, fmt.Errorf("%w: the payment date is in the past", domain.ErrInvalidArgument)
		case scheduledFor.After(today.AddDate(1, 0, 0)):
			return domain.Payment{}, fmt.Errorf("%w: payments can be scheduled up to a year ahead", domain.ErrInvalidArgument)
		case !b.DueDate.IsZero() && scheduledFor.After(domain.EffectiveDueDate(b.DueDate)):
			return domain.Payment{}, domain.Refuse("SCHEDULE_AFTER_DUE_DATE", "schedule the payment up to the due date")
		}
		date = scheduledFor
	}

	p := domain.Payment{ID: uuid.Must(uuid.NewV7()), IdempotencyKey: key, AccountID: account, Status: domain.StatusCreated,
		Barcode: b.Barcode, Line: b.Line, DueDate: b.DueDate, Nominal: nominal, ScheduledFor: date, CreatedAt: s.now()}
	if date.After(today) {
		p.Status = domain.StatusScheduled
	}
	q, err := s.quote(ctx, b, nominal, date)
	if r, ok := domain.AsRefused(err); ok {
		p.Status, p.FailureReason = domain.StatusRejected, r.Reason
	} else if err != nil {
		return domain.Payment{}, err
	} else {
		p.Apply(q)
	}

	fingerprint := account.String() + "|" + b.Barcode + "|" + strconv.FormatInt(nominal, 10) + "|" + date.Format(time.DateOnly)
	existing, fp, err := s.store.Insert(ctx, p, fingerprint)
	if err != nil {
		return domain.Payment{}, err
	}
	if existing != nil {
		if fp != fingerprint {
			return domain.Payment{}, fmt.Errorf("%w: %q", domain.ErrConflict, key)
		}
		return *existing, nil
	}
	switch p.Status {
	case domain.StatusCreated:
		return s.Process(ctx, p.ID)
	case domain.StatusScheduled:
		return p, s.save(ctx, p, "boleto.payment.scheduled")
	}
	return p, s.save(ctx, p, "boleto.payment.rejected")
}

func (s *Service) Payment(ctx context.Context, id uuid.UUID) (domain.Payment, error) {
	return s.store.Payment(ctx, id)
}

func (s *Service) Payments(ctx context.Context, account uuid.UUID) ([]domain.Payment, error) {
	return s.store.Payments(ctx, account, 100)
}

// Cancel cancels a scheduled payment.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID) (domain.Payment, error) {
	return s.withLease(ctx, id, func(p *domain.Payment) error {
		if p.Status == domain.StatusCancelled {
			return nil
		}
		if err := p.Cancel(); err != nil {
			return err
		}
		return s.save(ctx, *p, "boleto.payment.cancelled")
	})
}

// Process advances a payment under a lease. Every downstream call is
// idempotent (keyed by the payment id), so it can be re-run after any failure.
func (s *Service) Process(ctx context.Context, id uuid.UUID) (domain.Payment, error) {
	return s.withLease(ctx, id, func(p *domain.Payment) error { return s.run(ctx, p) })
}

func (s *Service) withLease(ctx context.Context, id uuid.UUID, fn func(*domain.Payment) error) (domain.Payment, error) {
	p, ok, err := s.store.Acquire(ctx, id, lease)
	if err != nil {
		return domain.Payment{}, err
	}
	if !ok {
		return p, fmt.Errorf("%w: %s", domain.ErrBusy, id)
	}
	runErr := fn(&p)
	bg := context.WithoutCancel(ctx)
	if err := s.store.Release(bg, id); err != nil {
		s.log.ErrorContext(ctx, "release lease", "payment_id", id, "error", err)
	}
	latest, err := s.store.Payment(bg, id)
	if err != nil {
		return p, errors.Join(runErr, err)
	}
	return latest, runErr
}

func (s *Service) run(ctx context.Context, p *domain.Payment) error {
	for !p.Status.Terminal() {
		var err error
		switch p.Status {
		case domain.StatusScheduled:
			if p.ScheduledFor.After(s.Today()) {
				return nil // not yet
			}
			err = s.requote(ctx, p)
		case domain.StatusCreated:
			err = s.reserve(ctx, p)
		case domain.StatusFundsReserved:
			p.Status = domain.StatusSentToClearing // record the intent before calling out
			err = s.store.Save(ctx, *p)
		case domain.StatusSentToClearing:
			err = s.settle(ctx, p)
		default:
			return fmt.Errorf("unexpected state %s", p.Status)
		}
		if err != nil {
			s.log.WarnContext(ctx, "boleto payment step failed, will retry", "payment_id", p.ID, "status", p.Status, "error", err)
			return err
		}
	}
	return nil
}

// requote prices a scheduled payment on its day (the payee may have changed
// or the boleto been paid elsewhere meanwhile).
func (s *Service) requote(ctx context.Context, p *domain.Payment) error {
	b, err := domain.ParseBoleto(p.Barcode, s.Today())
	if err != nil {
		return err
	}
	q, err := s.quote(ctx, b, p.Nominal, s.Today())
	if r, ok := domain.AsRefused(err); ok {
		return s.finish(ctx, p, domain.StatusRejected, r.Reason)
	}
	if err != nil {
		return err
	}
	p.Apply(q)
	p.Status = domain.StatusCreated
	return s.store.Save(ctx, *p)
}

func (s *Service) metadata(p *domain.Payment) map[string]string {
	return map[string]string{"boleto_payment_id": p.ID.String(), "counterparty_name": p.Payee.Name,
		"counterparty_bank": p.Payee.BankName, "barcode": p.Barcode}
}

// reserve consumes the limit and holds the amount.
func (s *Service) reserve(ctx context.Context, p *domain.Payment) error {
	res, err := s.txs.Reserve(ctx, p.SagaKey(), p.AccountID, p.Amount(), "Boleto "+p.Payee.Name, s.metadata(p))
	if err != nil {
		return err
	}
	p.TransactionID = res.ID
	switch res.Status {
	case "RESERVED":
		p.Status = domain.StatusFundsReserved
		return s.store.Save(ctx, *p)
	case "FAILED":
		return s.finish(ctx, p, domain.StatusRejected, res.FailureReason)
	}
	return fmt.Errorf("transaction %s still %s", res.ID, res.Status)
}

// settle pays the payee's bank, then captures or releases the hold.
func (s *Service) settle(ctx context.Context, p *domain.Payment) error {
	b, err := domain.ParseBoleto(p.Barcode, p.ScheduledFor)
	if err != nil {
		return err
	}
	err = s.clearing.Settle(ctx, p.ID, b, p.Amount())
	if r, ok := domain.AsRefused(err); ok {
		if _, err := s.txs.Compensate(ctx, p.TransactionID, r.Reason); err != nil {
			return err
		}
		return s.finish(ctx, p, domain.StatusFailed, r.Reason)
	}
	if err != nil {
		return err
	}
	if _, err := s.txs.Complete(ctx, p.TransactionID); err != nil {
		return err
	}
	return s.finish(ctx, p, domain.StatusCompleted, "")
}

func (s *Service) finish(ctx context.Context, p *domain.Payment, st domain.Status, reason string) error {
	p.Status, p.FailureReason = st, reason
	return s.save(ctx, *p, map[domain.Status]string{
		domain.StatusCompleted: "boleto.payment.completed",
		domain.StatusFailed:    "boleto.payment.failed",
		domain.StatusRejected:  "boleto.payment.rejected",
	}[st])
}

func (s *Service) save(ctx context.Context, p domain.Payment, eventType string) error {
	ev, err := outbox.NewEvent(ctx, source, eventType, map[string]any{
		"payment_id": p.ID, "account_id": p.AccountID, "status": p.Status, "failure_reason": p.FailureReason,
		"payee_name": p.Payee.Name, "amount": p.Amount(), "due_date": dateString(p.DueDate),
		"scheduled_for": dateString(p.ScheduledFor)})
	if err != nil {
		return err
	}
	return s.store.Save(ctx, p, outbox.Message{Topic: topic, Key: p.ID.String(), Event: ev})
}

func dateString(d time.Time) string {
	if d.IsZero() {
		return ""
	}
	return d.Format(time.DateOnly)
}

// RecoverOnce resumes payments stuck in flight and runs the scheduled ones
// whose day has come.
func (s *Service) RecoverOnce(ctx context.Context, staleAfter time.Duration) int {
	stuck, err := s.store.Unfinished(ctx, time.Now().Add(-staleAfter), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "unfinished payments", "error", err)
	}
	due, err := s.store.Due(ctx, s.Today(), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "due payments", "error", err)
	}
	n := 0
	for _, id := range append(stuck, due...) {
		if _, err := s.Process(ctx, id); err == nil {
			n++
		}
	}
	return n
}

func (s *Service) RunRecovery(ctx context.Context, interval, staleAfter time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RecoverOnce(ctx, staleAfter)
		}
	}
}

// SandboxIssuer stores sandbox boletos for the simulated registry.
type SandboxIssuer interface {
	InsertSandboxBoleto(ctx context.Context, barcode, payeeName, payeeDocument string) error
}

// IssueSandbox creates a valid boleto: behavior PAID makes the registry
// report it paid, REJECT makes the payee's bank refuse the settlement.
func IssueSandbox(ctx context.Context, issuer SandboxIssuer, amount int64, due time.Time, payee, behavior string) (domain.Boleto, error) {
	prefix := map[string]string{"": "", "PAID": "00000", "REJECT": "99999"}
	p, ok := prefix[behavior]
	if !ok {
		return domain.Boleto{}, fmt.Errorf("%w: behavior must be empty, PAID or REJECT", domain.ErrInvalidArgument)
	}
	free := domain.RandomFreeField(p)
	for p == "" && (free[:5] == "00000" || free[:5] == "99999") {
		free = domain.RandomFreeField(p)
	}
	b, err := domain.NewBoleto("237", due, amount, free)
	if err != nil {
		return domain.Boleto{}, err
	}
	if payee == "" {
		payee = "EMPRESA DE ENERGIA SANDBOX LTDA"
	}
	return b, issuer.InsertSandboxBoleto(ctx, b.Barcode, payee, "**.***.678/0001-**")
}
