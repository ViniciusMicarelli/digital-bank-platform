package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/ports"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/migrations"
)

// fakeLedger records created accounts and returns configurable balances.
type fakeLedger struct {
	mu       sync.Mutex
	created  map[uuid.UUID]int
	balances map[uuid.UUID]int64
	fail     error
}

func (f *fakeLedger) CreateCustomerAccount(_ context.Context, id uuid.UUID, _ money.Currency) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.created[id]++
	return nil
}

func (f *fakeLedger) Balance(_ context.Context, id uuid.UUID) (ports.Balance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, _ := money.New(f.balances[id], money.BRL)
	zero, _ := money.Zero(money.BRL)
	return ports.Balance{Posted: m, Pending: zero, Available: m}, nil
}

type clock struct{ t atomic.Pointer[time.Time] }

func (c *clock) now() time.Time { return *c.t.Load() }
func (c *clock) set(s string) {
	t, _ := time.ParseInLocation("2006-01-02 15:04", s, domain.Location)
	c.t.Store(&t)
}

func setup(t *testing.T) (*application.Service, *fakeLedger, *clock) {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	ledger := &fakeLedger{created: map[uuid.UUID]int{}, balances: map[uuid.UUID]int64{}}
	c := &clock{}
	c.set("2026-03-10 10:00")
	return application.New(pgstore.NewStore(pool), ledger, c.now), ledger, c
}

func open(t *testing.T, svc *application.Service, key string) domain.Account {
	t.Helper()
	a, err := svc.OpenAccount(context.Background(), key, "cust-1", "Ana Silva", "529.982.247-25", "")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func brl(s string) money.Money { return money.MustParse(s, money.BRL) }

func TestOpenAccountIdempotent(t *testing.T) {
	svc, ledger, _ := setup(t)
	ctx := context.Background()
	a := open(t, svc, "open-1")
	b := open(t, svc, "open-1")
	if a.ID != b.ID || a.Number != b.Number || a.Number == "" {
		t.Fatalf("retry returned a different account: %+v vs %+v", a, b)
	}
	if ledger.created[a.ID] != 2 {
		t.Errorf("ledger account creation should be retried (idempotent there), got %d calls", ledger.created[a.ID])
	}
	if _, err := svc.OpenAccount(ctx, "open-1", "cust-2", "Bruno", "111.444.777-35", ""); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("conflicting retry err = %v", err)
	}
	list, err := svc.AccountsByCustomer(ctx, "cust-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
}

func TestOpenAccountLedgerFailure(t *testing.T) {
	svc, ledger, _ := setup(t)
	ledger.fail = errors.New("ledger down")
	if _, err := svc.OpenAccount(context.Background(), "k", "c", "Ana Silva", "52998224725", ""); err == nil {
		t.Fatal("expected an error when the ledger is down")
	}
	ledger.fail = nil
	a := open(t, svc, "k") // retry after recovery succeeds
	if a.Status != domain.StatusActive {
		t.Fatalf("status = %s", a.Status)
	}
}

func TestLifecycleAndClose(t *testing.T) {
	svc, ledger, _ := setup(t)
	ctx := context.Background()
	a := open(t, svc, "k")

	if _, err := svc.Block(ctx, a.ID, "suspected fraud"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConsumeLimit(ctx, a.ID, "tx-1", brl("1")); !errors.Is(err, domain.ErrNotActive) {
		t.Fatalf("blocked account must not move money: %v", err)
	}
	if _, err := svc.Unblock(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	ledger.balances[a.ID] = 100
	if _, err := svc.Close(ctx, a.ID, ""); !errors.Is(err, domain.ErrBalanceNotZero) {
		t.Fatalf("close with balance err = %v", err)
	}
	ledger.balances[a.ID] = 0
	closed, err := svc.Close(ctx, a.ID, "customer request")
	if err != nil || closed.Status != domain.StatusClosed || closed.ClosedAt == nil {
		t.Fatalf("close = %+v, %v", closed, err)
	}
	if _, err := svc.UpdateLimits(ctx, a.ID, domain.DefaultLimits); !errors.Is(err, domain.ErrNotActive) {
		t.Fatalf("update limits on closed account err = %v", err)
	}
}

func TestConsumeAndRestoreLimits(t *testing.T) {
	svc, _, clk := setup(t)
	ctx := context.Background()
	a := open(t, svc, "k")
	if _, err := svc.UpdateLimits(ctx, a.ID, domain.Limits{PerTransaction: 10_000, Daily: 25_000, Nightly: 5_000}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ConsumeLimit(ctx, a.ID, "t1", brl("100.01")); !errors.Is(err, domain.ErrLimitExceeded) {
		t.Fatalf("per-transaction err = %v", err)
	}
	for i := range 2 {
		if _, err := svc.ConsumeLimit(ctx, a.ID, fmt.Sprintf("d%d", i), brl("100")); err != nil {
			t.Fatal(err)
		}
	}
	// Retrying d0 does not count twice.
	u, err := svc.ConsumeLimit(ctx, a.ID, "d0", brl("100"))
	if err != nil || u.Daily != 20_000 {
		t.Fatalf("idempotent retry usage = %+v, %v", u, err)
	}
	if _, err := svc.ConsumeLimit(ctx, a.ID, "d2", brl("50.01")); !errors.Is(err, domain.ErrLimitExceeded) {
		t.Fatalf("daily err = %v", err)
	}
	if err := svc.RestoreLimit(ctx, a.ID, "d1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConsumeLimit(ctx, a.ID, "d3", brl("100")); err != nil {
		t.Fatalf("after restore: %v", err)
	}

	// Night (20:00-06:00): the nightly limit (R$ 50) applies and spans midnight.
	clk.set("2026-03-10 22:00")
	if _, err := svc.ConsumeLimit(ctx, a.ID, "n1", brl("40")); err != nil {
		t.Fatal(err)
	}
	clk.set("2026-03-11 01:00") // new day for the daily limit, same night
	if _, err := svc.ConsumeLimit(ctx, a.ID, "n2", brl("10.01")); !errors.Is(err, domain.ErrLimitExceeded) {
		t.Fatalf("nightly limit must carry over midnight: %v", err)
	}
	if _, err := svc.ConsumeLimit(ctx, a.ID, "n3", brl("10")); err != nil {
		t.Fatal(err)
	}
	clk.set("2026-03-11 09:00") // daytime: nightly no longer applies
	if _, err := svc.ConsumeLimit(ctx, a.ID, "d4", brl("50")); err != nil {
		t.Fatal(err)
	}
	_, u, err = svc.Limits(ctx, a.ID)
	if err != nil || u.Daily != 6_000 { // n3 (01:00) + d4, both on 2026-03-11
		t.Fatalf("usage on 2026-03-11 = %+v, %v", u, err)
	}
}

func TestConcurrentConsumeRespectsDailyLimit(t *testing.T) {
	svc, _, _ := setup(t)
	ctx := context.Background()
	a := open(t, svc, "k")
	if _, err := svc.UpdateLimits(ctx, a.ID, domain.Limits{PerTransaction: 1_000, Daily: 10_000, Nightly: 1_000}); err != nil {
		t.Fatal(err)
	}
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := range 30 {
		wg.Go(func() {
			if _, err := svc.ConsumeLimit(ctx, a.ID, fmt.Sprintf("c%d", i), brl("10")); err == nil {
				ok.Add(1)
			} else if !errors.Is(err, domain.ErrLimitExceeded) {
				t.Errorf("unexpected: %v", err)
			}
		})
	}
	wg.Wait()
	if ok.Load() != 10 {
		t.Fatalf("%d transactions passed a limit that allows 10", ok.Load())
	}
}
