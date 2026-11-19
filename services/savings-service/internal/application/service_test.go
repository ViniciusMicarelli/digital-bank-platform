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
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/ports"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/migrations"
)

type posting struct {
	account uuid.UUID
	amount  int64
	at      time.Time
}

// fakeBank is the ledger and transaction-service: balances with history,
// no account below zero except the bank's yield account, idempotent keys.
type fakeBank struct {
	mu       sync.Mutex
	now      *time.Time
	postings []posting
	byKey    map[string]ports.TxResult
	down     bool
	accounts map[uuid.UUID]bool
}

func (f *fakeBank) CreateBoxAccount(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[id] = true
	return nil
}

func (f *fakeBank) balance(id uuid.UUID, at time.Time) int64 {
	var b int64
	for _, p := range f.postings {
		if p.account == id && !p.at.After(at) {
			b += p.amount
		}
	}
	return b
}

func (f *fakeBank) Balance(_ context.Context, id uuid.UUID) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.balance(id, time.Unix(1<<40, 0)), nil
}

func (f *fakeBank) BalanceAt(_ context.Context, id uuid.UUID, at time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.balance(id, at), nil
}

func (f *fakeBank) Post(_ context.Context, key, _ string, from, to uuid.UUID, amount int64, _ string, _ map[string]string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return ports.TxResult{}, errors.New("transaction-service unavailable")
	}
	if r, ok := f.byKey[key]; ok {
		return r, nil
	}
	r := ports.TxResult{Status: "COMPLETED"}
	if from != application.SavingsYieldAccountID && f.balance(from, time.Unix(1<<40, 0)) < amount {
		r = ports.TxResult{Status: "FAILED", FailureReason: "INSUFFICIENT_FUNDS"}
	} else {
		f.postings = append(f.postings, posting{from, -amount, *f.now}, posting{to, amount, *f.now})
	}
	f.byKey[key] = r
	return r, nil
}

type env struct {
	ctx     context.Context
	svc     *application.Service
	bank    *fakeBank
	account uuid.UUID
	now     time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	e := &env{ctx: context.Background(), account: uuid.New(), now: time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)}
	e.bank = &fakeBank{now: &e.now, byKey: map[string]ports.TxResult{}, accounts: map[uuid.UUID]bool{}}
	e.bank.postings = append(e.bank.postings, posting{e.account, 1_000_00, e.now.Add(-time.Hour)}) // R$ 1.000 in checking
	e.svc = application.New(pgstore.NewStore(pool), e.bank, e.bank, slog.New(slog.NewTextHandler(io.Discard, nil)), 1065)
	e.svc.SetClock(func() time.Time { return e.now })
	return e
}

func reason(err error) string {
	if r, ok := domain.AsRefused(err); ok {
		return r.Reason
	}
	return ""
}

func TestBoxesAndMovements(t *testing.T) {
	e := setup(t)
	b, err := e.svc.Create(e.ctx, "k1", e.account, "Viagem", 3_000_00)
	if err != nil || b.Name != "Viagem" || !e.bank.accounts[b.ID] {
		t.Fatalf("create = %+v %v", b, err)
	}
	if again, _ := e.svc.Create(e.ctx, "k1", e.account, "Viagem", 3_000_00); again.ID != b.ID {
		t.Fatal("same key, same box")
	}
	if _, err := e.svc.Create(e.ctx, "k1", uuid.New(), "Outra", 0); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("same key, other account: %v", err)
	}

	m, v, err := e.svc.Deposit(e.ctx, "d1", b.ID, 400_00)
	if err != nil || m.Status != domain.Completed || v.Balance != 400_00 {
		t.Fatalf("deposit = %+v %+v %v", m, v, err)
	}
	if again, _, _ := e.svc.Deposit(e.ctx, "d1", b.ID, 400_00); again.ID != m.ID {
		t.Fatal("a retried deposit moves money once")
	}
	if m, _, _ := e.svc.Deposit(e.ctx, "d2", b.ID, 700_00); m.Status != domain.Failed || m.FailureReason != "INSUFFICIENT_FUNDS" {
		t.Fatalf("more than the checking account has = %+v", m)
	}
	if m, _, _ := e.svc.Withdraw(e.ctx, "w1", b.ID, 500_00); m.Status != domain.Failed {
		t.Fatalf("withdraw more than the box has = %+v", m)
	}
	if _, v, _ := e.svc.Withdraw(e.ctx, "w2", b.ID, 150_00); v.Balance != 250_00 {
		t.Fatalf("after a withdrawal = %+v", v)
	}

	// An outage: the deposit waits and the recovery loop finishes it.
	e.bank.down = true
	m, _, _ = e.svc.Deposit(e.ctx, "d3", b.ID, 50_00)
	if m.Status != domain.Pending {
		t.Fatalf("during the outage = %+v", m)
	}
	e.bank.down = false
	e.now = e.now.Add(time.Minute)
	e.svc.RecoverOnce(e.ctx, 30*time.Second)
	if v, _ := e.svc.Box(e.ctx, b.ID); v.Balance != 300_00 {
		t.Fatalf("after recovery = %+v", v)
	}

	// Closing returns everything to the checking account.
	closed, err := e.svc.Close(e.ctx, "c1", b.ID)
	if err != nil || closed.Status != domain.Closed || closed.Balance != 0 {
		t.Fatalf("close = %+v %v", closed, err)
	}
	if bal, _ := e.bank.Balance(e.ctx, e.account); bal != 1_000_00 {
		t.Fatalf("checking after closing = %d", bal)
	}
	if _, _, err := e.svc.Deposit(e.ctx, "d4", b.ID, 1_00); reason(err) != "BOX_CLOSED" {
		t.Fatalf("deposit into a closed box: %v", err)
	}
	if list, _ := e.svc.Boxes(e.ctx, e.account); len(list) != 0 {
		t.Fatalf("closed boxes are not listed: %+v", list)
	}
}

func TestDailyYield(t *testing.T) {
	e := setup(t)
	b, _ := e.svc.Create(e.ctx, "k1", e.account, "Reserva", 0)
	_, _, _ = e.svc.Deposit(e.ctx, "d1", b.ID, 1_000_00) // all of it, on the 18th

	// Nothing on the day it is created.
	e.svc.AccrueOnce(e.ctx)
	if v, _ := e.svc.Box(e.ctx, b.ID); v.Yielded != 0 {
		t.Fatalf("same day = %+v", v)
	}
	// Five days later: the 19th..22nd are paid, each on that day's closing balance.
	e.now = e.now.AddDate(0, 0, 5)
	e.svc.AccrueOnce(e.ctx)
	v, _ := e.svc.Box(e.ctx, b.ID)
	want := int64(0)
	bal, carry := int64(1_000_00), int64(0)
	for range 4 {
		var a int64
		a, carry = domain.Accrue(bal, 1065, 1, carry)
		bal += a
		want += a
	}
	if v.Yielded != want || v.Balance != 1_000_00+want || want == 0 {
		t.Fatalf("yield = %d (balance %d), want %d", v.Yielded, v.Balance, want)
	}
	// Running again pays nothing twice.
	e.svc.AccrueOnce(e.ctx)
	if again, _ := e.svc.Box(e.ctx, b.ID); again.Yielded != want {
		t.Fatalf("accrued twice: %d", again.Yielded)
	}
	list, _ := e.svc.Movements(e.ctx, b.ID, 10)
	if len(list) != 5 || list[0].Kind != domain.Yield {
		t.Fatalf("movements = %+v", list)
	}

	// Sandbox: a year at once.
	m, v, err := e.svc.SimulateYield(e.ctx, b.ID, 365)
	if err != nil || m.Amount < 100_00 || v.Yielded != want+m.Amount {
		t.Fatalf("simulated year = %+v %+v %v", m, v, err)
	}
}
