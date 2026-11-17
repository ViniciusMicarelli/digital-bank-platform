package application_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/migrations"
)

func newService(t *testing.T) (*application.Service, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return application.New(pgstore.NewStore(pool), log), pool
}

func brl(s string) money.Money { return money.MustParse(s, money.BRL) }

func neg(m money.Money) money.Money { n, _ := m.Neg(); return n }

func customer(t *testing.T, svc *application.Service, overdraft int64) uuid.UUID {
	t.Helper()
	acc, err := svc.CreateAccount(context.Background(), domain.Account{
		ID: uuid.New(), Kind: domain.KindCustomer, Currency: money.BRL, OverdraftLimit: overdraft,
	})
	if err != nil {
		t.Fatal(err)
	}
	return acc.ID
}

// fund moves money from the bank funding account into a customer account.
func fund(t *testing.T, svc *application.Service, account uuid.UUID, amount string) {
	t.Helper()
	m := brl(amount)
	_, err := svc.PostEntry(context.Background(), "fund-"+uuid.NewString(), []domain.Posting{
		{AccountID: domain.BankFundingAccountID, Amount: neg(m)},
		{AccountID: account, Amount: m},
	}, "funding", nil)
	if err != nil {
		t.Fatal(err)
	}
}

func transfer(svc *application.Service, key string, from, to uuid.UUID, amount money.Money) (domain.Entry, error) {
	return svc.PostEntry(context.Background(), key, []domain.Posting{
		{AccountID: from, Amount: neg(amount)},
		{AccountID: to, Amount: amount},
	}, "transfer", nil)
}

func balance(t *testing.T, svc *application.Service, id uuid.UUID) domain.Balance {
	t.Helper()
	_, b, err := svc.Balance(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCreateAccountIdempotent(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	acc := domain.Account{ID: uuid.New(), Kind: domain.KindCustomer, Currency: money.BRL}
	first, err := svc.CreateAccount(ctx, acc)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.CreateAccount(ctx, acc)
	if err != nil || again.ID != first.ID {
		t.Fatalf("retry = %v, %v", again, err)
	}
	acc.OverdraftLimit = 100
	if _, err := svc.CreateAccount(ctx, acc); !errors.Is(err, domain.ErrAccountConflict) {
		t.Fatalf("different attributes err = %v", err)
	}
}

func TestPostEntryIdempotency(t *testing.T) {
	svc, _ := newService(t)
	a, b := customer(t, svc, 0), customer(t, svc, 0)
	fund(t, svc, a, "100")

	first, err := transfer(svc, "tx-1", a, b, brl("30"))
	if err != nil {
		t.Fatal(err)
	}
	retry, err := transfer(svc, "tx-1", a, b, brl("30"))
	if err != nil || retry.ID != first.ID {
		t.Fatalf("retry = %v, %v", retry.ID, err)
	}
	if got := balance(t, svc, a).Posted; got != 7000 {
		t.Fatalf("retry must not post twice: balance = %d", got)
	}
	if _, err := transfer(svc, "tx-1", a, b, brl("31")); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("same key, different amount err = %v", err)
	}
}

func TestInsufficientFunds(t *testing.T) {
	svc, _ := newService(t)
	a, b := customer(t, svc, 0), customer(t, svc, 2000)
	fund(t, svc, a, "10")
	if _, err := transfer(svc, "k1", a, b, brl("10.01")); !errors.Is(err, domain.ErrInsufficientFunds) {
		t.Fatalf("err = %v", err)
	}
	// The failed attempt must not consume the key or leave postings behind.
	if _, err := transfer(svc, "k1", a, b, brl("10")); err != nil {
		t.Fatalf("key should be reusable after a rolled back attempt: %v", err)
	}
	// b has a R$ 20 overdraft limit.
	if _, err := transfer(svc, "k2", b, a, brl("30")); err != nil {
		t.Fatalf("within overdraft: %v", err)
	}
	if got := balance(t, svc, b).Posted; got != -2000 {
		t.Errorf("b = %d, want -2000", got)
	}
}

func TestUnknownAccountAndCurrency(t *testing.T) {
	svc, _ := newService(t)
	a := customer(t, svc, 0)
	if _, err := transfer(svc, "k", domain.BankFundingAccountID, uuid.New(), brl("1")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown account err = %v", err)
	}
	usd := money.MustParse("1", money.USD)
	if _, err := transfer(svc, "k2", domain.BankFundingAccountID, a, usd); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("currency mismatch err = %v", err)
	}
}

func TestReverseEntry(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	a, b := customer(t, svc, 0), customer(t, svc, 0)
	fund(t, svc, a, "50")
	e, _ := transfer(svc, "pay", a, b, brl("20"))

	rev, err := svc.ReverseEntry(ctx, "pay-rev", e.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if rev.ReversesEntryID != e.ID {
		t.Errorf("reverses = %v", rev.ReversesEntryID)
	}
	if balance(t, svc, a).Posted != 5000 || balance(t, svc, b).Posted != 0 {
		t.Error("reversal did not restore balances")
	}
	if again, err := svc.ReverseEntry(ctx, "pay-rev", e.ID, ""); err != nil || again.ID != rev.ID {
		t.Errorf("retry = %v, %v", again.ID, err)
	}
	if _, err := svc.ReverseEntry(ctx, "pay-rev-2", e.ID, ""); !errors.Is(err, domain.ErrAlreadyReversed) {
		t.Errorf("second reversal err = %v", err)
	}
	if _, err := svc.ReverseEntry(ctx, "x", uuid.New(), ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown entry err = %v", err)
	}
}

func TestHoldCaptureRelease(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	a := customer(t, svc, 0)
	fund(t, svc, a, "100")
	settlement := domain.PixSettlementAccountID

	h, err := svc.Hold(ctx, "hold-1", a, settlement, brl("60"), "pix out")
	if err != nil {
		t.Fatal(err)
	}
	if b := balance(t, svc, a); b.Pending != 6000 || b.Available() != 4000 {
		t.Fatalf("after hold: %+v", b)
	}
	if again, err := svc.Hold(ctx, "hold-1", a, settlement, brl("60"), "pix out"); err != nil || again.ID != h.ID {
		t.Fatalf("hold retry = %v, %v", again.ID, err)
	}
	if _, err := svc.Hold(ctx, "hold-2", a, settlement, brl("40.01"), ""); !errors.Is(err, domain.ErrInsufficientFunds) {
		t.Fatalf("hold beyond available err = %v", err)
	}

	partial := brl("55")
	captured, entry, err := svc.Capture(ctx, h.ID, &partial)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Status != domain.HoldCaptured || captured.CaptureEntryID != entry.ID {
		t.Fatalf("capture = %+v", captured)
	}
	if b := balance(t, svc, a); b.Posted != 4500 || b.Pending != 0 {
		t.Fatalf("after partial capture: %+v", b)
	}
	if _, again, err := svc.Capture(ctx, h.ID, nil); err != nil || again.ID != entry.ID {
		t.Fatalf("capture retry = %v, %v", again.ID, err)
	}
	if _, err := svc.Release(ctx, h.ID); !errors.Is(err, domain.ErrHoldNotActive) {
		t.Fatalf("release captured err = %v", err)
	}

	h2, _ := svc.Hold(ctx, "hold-3", a, settlement, brl("45"), "")
	if _, err := svc.Release(ctx, h2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Release(ctx, h2.ID); err != nil {
		t.Fatalf("release is idempotent: %v", err)
	}
	if b := balance(t, svc, a); b.Available() != 4500 {
		t.Fatalf("after release: %+v", b)
	}
}

func TestBalanceAtAndPostings(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	a := customer(t, svc, 0)
	fund(t, svc, a, "10")
	mid := time.Now()
	time.Sleep(20 * time.Millisecond)
	fund(t, svc, a, "5")

	_, before, err := svc.BalanceAt(ctx, a, mid)
	if err != nil || before != 1000 {
		t.Fatalf("balance at mid = %d, %v", before, err)
	}
	page, err := svc.Postings(ctx, a, 0, 1)
	if err != nil || len(page) != 1 || page[0].Amount != 500 {
		t.Fatalf("newest posting = %+v, %v", page, err)
	}
	next, _ := svc.Postings(ctx, a, page[0].ID, 10)
	if len(next) != 1 || next[0].Amount != 1000 {
		t.Fatalf("second page = %+v", next)
	}
}

func TestDatabaseRejectsUnbalancedAndMutations(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	a := customer(t, svc, 0)
	fund(t, svc, a, "1")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	_, _ = tx.Exec(ctx, `INSERT INTO journal_entries (id, idempotency_key, fingerprint) VALUES ($1, 'raw', '\x00')`, id)
	_, _ = tx.Exec(ctx, `INSERT INTO postings (entry_id, account_id, amount, currency) VALUES ($1, $2, 100, 'BRL')`, id, a)
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("database accepted an unbalanced entry")
	}
	if _, err := pool.Exec(ctx, `UPDATE postings SET amount = amount * 2`); err == nil {
		t.Fatal("database allowed updating postings")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM journal_entries`); err == nil {
		t.Fatal("database allowed deleting entries")
	}
}

func TestEventsWrittenToOutbox(t *testing.T) {
	svc, pool := newService(t)
	a := customer(t, svc, 0)
	fund(t, svc, a, "1")
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbox WHERE payload->>'type' = 'ledger.entry.posted'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("outbox events = %d, %v", n, err)
	}
}

// N concurrent debits against one account must never overdraw it.
func TestConcurrentDebitsNeverOverdraw(t *testing.T) {
	svc, _ := newService(t)
	a := customer(t, svc, 0)
	fund(t, svc, a, "100")

	const workers = 40
	var ok, insufficient atomic.Int32
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			to := domain.PixSettlementAccountID
			_, err := transfer(svc, fmt.Sprintf("debit-%d", i), a, to, brl("10"))
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, domain.ErrInsufficientFunds):
				insufficient.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()
	if ok.Load() != 10 || insufficient.Load() != workers-10 {
		t.Fatalf("ok=%d insufficient=%d, want 10/%d", ok.Load(), insufficient.Load(), workers-10)
	}
	if b := balance(t, svc, a); b.Posted != 0 {
		t.Fatalf("final balance = %d, want 0", b.Posted)
	}
}

// Random concurrent operations; afterwards every invariant must hold.
func TestRandomOperationsKeepInvariants(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	accounts := make([]uuid.UUID, 6)
	for i := range accounts {
		accounts[i] = customer(t, svc, int64(i%2)*1000) // half have R$ 10 overdraft
		fund(t, svc, accounts[i], "50")
	}
	pick := func(r *rand.Rand) (uuid.UUID, uuid.UUID) {
		i := r.IntN(len(accounts))
		j := (i + 1 + r.IntN(len(accounts)-1)) % len(accounts)
		return accounts[i], accounts[j]
	}

	var wg sync.WaitGroup
	for w := range 8 {
		wg.Go(func() {
			r := rand.New(rand.NewPCG(uint64(w), 42))
			for i := range 40 {
				from, to := pick(r)
				amount, _ := money.New(int64(1+r.IntN(3000)), money.BRL)
				key := fmt.Sprintf("w%d-%d", w, i)
				var err error
				switch r.IntN(4) {
				case 0, 1:
					_, err = transfer(svc, key, from, to, amount)
				case 2:
					var h domain.Hold
					if h, err = svc.Hold(ctx, key, from, to, amount, ""); err == nil {
						if r.IntN(2) == 0 {
							_, _, err = svc.Capture(ctx, h.ID, nil)
						} else {
							_, err = svc.Release(ctx, h.ID)
						}
					}
				case 3:
					var e domain.Entry
					if e, err = transfer(svc, key, from, to, amount); err == nil {
						_, err = svc.ReverseEntry(ctx, key+"-rev", e.ID, "")
					}
				}
				if err != nil && !errors.Is(err, domain.ErrInsufficientFunds) {
					t.Errorf("op %s: %v", key, err)
				}
			}
		})
	}
	wg.Wait()

	checks := map[string]string{
		"global sum of postings is zero": `SELECT COALESCE(SUM(amount), 0) <> 0 FROM postings`,
		"posted equals sum of postings": `SELECT EXISTS (
			SELECT 1 FROM balances b
			 WHERE b.posted <> (SELECT COALESCE(SUM(amount), 0) FROM postings p WHERE p.account_id = b.account_id))`,
		"pending equals sum of active holds": `SELECT EXISTS (
			SELECT 1 FROM balances b
			 WHERE b.pending <> (SELECT COALESCE(SUM(amount), 0) FROM holds h
			                      WHERE h.account_id = b.account_id AND h.status = 'ACTIVE'))`,
		"no customer below its floor": `SELECT EXISTS (
			SELECT 1 FROM balances b JOIN ledger_accounts a ON a.id = b.account_id
			 WHERE a.kind = 'CUSTOMER' AND b.posted - b.pending < -a.overdraft_limit)`,
		"every entry balances": `SELECT EXISTS (
			SELECT 1 FROM postings GROUP BY entry_id HAVING SUM(amount) <> 0)`,
	}
	for name, q := range checks {
		var violated bool
		if err := pool.QueryRow(ctx, q).Scan(&violated); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if violated {
			t.Errorf("invariant violated: %s", name)
		}
	}
}
