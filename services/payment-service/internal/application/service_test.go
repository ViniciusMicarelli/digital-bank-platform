package application_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/adapters/simulator"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/ports"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/migrations"
)

// fakeTxs holds and captures money per account, idempotent on the key.
type fakeTxs struct {
	mu      sync.Mutex
	balance map[uuid.UUID]int64
	byKey   map[string]ports.TxResult
	held    map[uuid.UUID]struct {
		account uuid.UUID
		amount  int64
	}
	down bool
}

func (f *fakeTxs) Reserve(_ context.Context, key string, account uuid.UUID, amount int64, _ string, _ map[string]string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return ports.TxResult{}, errors.New("transaction-service unavailable")
	}
	if r, ok := f.byKey[key]; ok {
		return r, nil
	}
	r := ports.TxResult{ID: uuid.New(), Status: "RESERVED"}
	if f.balance[account] < amount {
		r.Status, r.FailureReason = "FAILED", "INSUFFICIENT_FUNDS"
	} else {
		f.balance[account] -= amount
		f.held[r.ID] = struct {
			account uuid.UUID
			amount  int64
		}{account, amount}
	}
	f.byKey[key] = r
	return r, nil
}

func (f *fakeTxs) Complete(_ context.Context, id uuid.UUID) (ports.TxResult, error) {
	return ports.TxResult{ID: id, Status: "COMPLETED"}, nil
}

func (f *fakeTxs) Compensate(_ context.Context, id uuid.UUID, _ string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.held[id]
	f.balance[h.account] += h.amount
	delete(f.held, id)
	return ports.TxResult{ID: id, Status: "FAILED"}, nil
}

type env struct {
	ctx     context.Context
	svc     *application.Service
	store   *pgstore.Store
	txs     *fakeTxs
	account uuid.UUID
	now     time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	store := pgstore.NewStore(pool)
	e := &env{ctx: context.Background(), store: store, account: uuid.New(),
		now: time.Date(2026, 11, 17, 15, 0, 0, 0, time.UTC),
		txs: &fakeTxs{balance: map[uuid.UUID]int64{}, byKey: map[string]ports.TxResult{}, held: map[uuid.UUID]struct {
			account uuid.UUID
			amount  int64
		}{}}}
	e.svc = application.New(store, simulator.NewRegistry(store), simulator.NewClearing(), e.txs, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.svc.SetClock(func() time.Time { return e.now })
	e.txs.balance[e.account] = 1_000_00
	return e
}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func (e *env) boleto(t *testing.T, amount int64, due time.Time, behavior string) string {
	t.Helper()
	b, err := application.IssueSandbox(e.ctx, e.store, amount, due, "CIA DE AGUA", behavior)
	if err != nil {
		t.Fatal(err)
	}
	return domain.FormatLine(b.Line)
}

func reason(err error) string {
	if r, ok := domain.AsRefused(err); ok {
		return r.Reason
	}
	return ""
}

func TestLookup(t *testing.T) {
	e := setup(t)
	q, err := e.svc.Lookup(e.ctx, e.boleto(t, 200_00, date(2026, 11, 2), ""))
	if err != nil || q.Payee.Name != "CIA DE AGUA" || q.Payee.BankName != "Banco Bradesco S.A." {
		t.Fatalf("quote = %+v %v", q, err)
	}
	if q.Fine != 4_00 || q.Interest != 1_00 || q.Total() != 205_00 { // 15 days late
		t.Fatalf("overdue quote = %+v", q)
	}
	if _, err := e.svc.Lookup(e.ctx, e.boleto(t, 10_00, date(2026, 12, 1), "PAID")); reason(err) != "BOLETO_ALREADY_PAID" {
		t.Fatalf("paid boleto: %v", err)
	}
	if _, err := e.svc.Lookup(e.ctx, "12345"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("garbage: %v", err)
	}
}

func TestPayNow(t *testing.T) {
	e := setup(t)
	line := e.boleto(t, 150_00, date(2026, 12, 1), "")
	p, err := e.svc.Pay(e.ctx, "k1", e.account, line, 0, time.Time{})
	if err != nil || p.Status != domain.StatusCompleted || p.Amount() != 150_00 || e.txs.balance[e.account] != 850_00 {
		t.Fatalf("pay = %+v %v (balance %d)", p, err, e.txs.balance[e.account])
	}
	if again, err := e.svc.Pay(e.ctx, "k1", e.account, line, 0, time.Time{}); err != nil || again.ID != p.ID || e.txs.balance[e.account] != 850_00 {
		t.Fatalf("retry = %+v %v", again, err)
	}
	if _, err := e.svc.Pay(e.ctx, "k2", e.account, line, 0, time.Time{}); reason(err) != "DUPLICATE_PAYMENT" {
		t.Fatalf("paying a boleto twice: %v", err)
	}

	// Not enough money: rejected, and the boleto can be paid later.
	big := e.boleto(t, 2_000_00, date(2026, 12, 1), "")
	if p, _ := e.svc.Pay(e.ctx, "k3", e.account, big, 0, time.Time{}); p.Status != domain.StatusRejected || p.FailureReason != "INSUFFICIENT_FUNDS" {
		t.Fatalf("no money = %+v", p)
	}
	e.txs.balance[e.account] += 2_000_00
	if p, _ := e.svc.Pay(e.ctx, "k4", e.account, big, 0, time.Time{}); p.Status != domain.StatusCompleted {
		t.Fatalf("after funding = %+v", p)
	}

	// The payee's bank refuses: the hold is released.
	before := e.txs.balance[e.account]
	if p, _ := e.svc.Pay(e.ctx, "k5", e.account, e.boleto(t, 50_00, date(2026, 12, 1), "REJECT"), 0, time.Time{}); p.Status != domain.StatusFailed ||
		p.FailureReason != "SETTLEMENT_REJECTED" || e.txs.balance[e.account] != before {
		t.Fatalf("rejected by the payee's bank = %+v (balance %d, was %d)", p, e.txs.balance[e.account], before)
	}

	// Open amount: the payer says how much.
	open := e.boleto(t, 0, time.Time{}, "")
	if _, err := e.svc.Pay(e.ctx, "k6", e.account, open, 0, time.Time{}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatal("an open-amount boleto needs an amount")
	}
	if p, _ := e.svc.Pay(e.ctx, "k6", e.account, open, 30_00, time.Time{}); p.Status != domain.StatusCompleted || p.Amount() != 30_00 {
		t.Fatalf("open amount = %+v", p)
	}
}

func TestScheduled(t *testing.T) {
	e := setup(t)
	line := e.boleto(t, 100_00, date(2026, 11, 30), "")
	if _, err := e.svc.Pay(e.ctx, "k0", e.account, line, 0, date(2026, 12, 1)); reason(err) != "SCHEDULE_AFTER_DUE_DATE" {
		t.Fatalf("after the due date: %v", err)
	}
	p, err := e.svc.Pay(e.ctx, "k1", e.account, line, 0, date(2026, 11, 25))
	if err != nil || p.Status != domain.StatusScheduled || e.txs.balance[e.account] != 1_000_00 {
		t.Fatalf("schedule = %+v %v", p, err)
	}
	other := e.boleto(t, 40_00, date(2026, 11, 30), "")
	q, _ := e.svc.Pay(e.ctx, "k2", e.account, other, 0, date(2026, 11, 20))
	if c, err := e.svc.Cancel(e.ctx, q.ID); err != nil || c.Status != domain.StatusCancelled {
		t.Fatalf("cancel = %+v %v", c, err)
	}

	e.svc.RecoverOnce(e.ctx, time.Second)
	if p, _ := e.svc.Payment(e.ctx, p.ID); p.Status != domain.StatusScheduled {
		t.Fatal("ran before its day")
	}
	e.now = time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	e.svc.RecoverOnce(e.ctx, time.Second)
	if p, _ := e.svc.Payment(e.ctx, p.ID); p.Status != domain.StatusCompleted || e.txs.balance[e.account] != 900_00 {
		t.Fatalf("on its day = %+v", p)
	}
	if _, err := e.svc.Cancel(e.ctx, p.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatal("a paid boleto cannot be cancelled")
	}
}

func TestRecovery(t *testing.T) {
	e := setup(t)
	e.txs.down = true
	line := e.boleto(t, 70_00, date(2026, 12, 1), "")
	if _, err := e.svc.Pay(e.ctx, "k", e.account, line, 0, time.Time{}); err == nil {
		t.Fatal("expected the outage to surface")
	}
	list, _ := e.svc.Payments(e.ctx, e.account)
	if len(list) != 1 || list[0].Status != domain.StatusCreated {
		t.Fatalf("after the outage = %+v", list)
	}
	e.txs.down = false
	e.svc.RecoverOnce(e.ctx, -time.Second)
	if p, _ := e.svc.Payment(e.ctx, list[0].ID); p.Status != domain.StatusCompleted {
		t.Fatalf("recovered = %+v", p)
	}
}
