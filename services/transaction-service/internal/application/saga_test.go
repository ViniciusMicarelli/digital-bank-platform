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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/migrations"
)

var errDown = errors.New("connection refused")

// fakeLedger keeps balances in memory and honours idempotency keys.
type fakeLedger struct {
	mu       sync.Mutex
	balances map[uuid.UUID]int64
	entries  map[string]uuid.UUID // key -> entry id
	holds    map[uuid.UUID]string // id -> ACTIVE/CAPTURED/RELEASED
	holdKeys map[string]uuid.UUID
	holdAmt  map[uuid.UUID]int64
	down     bool
	// lostResponse makes the next Transfer succeed but report a failure.
	lostResponse bool
}

func newLedger() *fakeLedger {
	return &fakeLedger{balances: map[uuid.UUID]int64{}, entries: map[string]uuid.UUID{},
		holds: map[uuid.UUID]string{}, holdKeys: map[string]uuid.UUID{}, holdAmt: map[uuid.UUID]int64{}}
}

func (l *fakeLedger) Transfer(_ context.Context, key string, from, to uuid.UUID, amount money.Money, _ string, _ map[string]string) (uuid.UUID, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.down {
		return uuid.Nil, errDown
	}
	if id, ok := l.entries[key]; ok {
		return id, nil
	}
	if l.balances[from] < amount.Minor() {
		return uuid.Nil, domain.Reject("INSUFFICIENT_FUNDS", "not enough money")
	}
	l.balances[from] -= amount.Minor()
	l.balances[to] += amount.Minor()
	id := uuid.New()
	l.entries[key] = id
	if l.lostResponse {
		l.lostResponse = false
		return uuid.Nil, errDown
	}
	return id, nil
}

func (l *fakeLedger) Hold(_ context.Context, key string, from, _ uuid.UUID, amount money.Money, _ string) (uuid.UUID, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.down {
		return uuid.Nil, errDown
	}
	if id, ok := l.holdKeys[key]; ok {
		return id, nil
	}
	if l.balances[from] < amount.Minor() {
		return uuid.Nil, domain.Reject("INSUFFICIENT_FUNDS", "not enough money")
	}
	l.balances[from] -= amount.Minor() // simplified: held money is not available
	id := uuid.New()
	l.holdKeys[key], l.holds[id], l.holdAmt[id] = id, "ACTIVE", amount.Minor()
	return id, nil
}

func (l *fakeLedger) Capture(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.down {
		return uuid.Nil, errDown
	}
	if l.holds[id] != "ACTIVE" && l.holds[id] != "CAPTURED" {
		return uuid.Nil, domain.Reject("HOLD_NOT_ACTIVE", "hold is "+l.holds[id])
	}
	l.holds[id] = "CAPTURED"
	return uuid.NewSHA1(uuid.Nil, id[:]), nil
}

func (l *fakeLedger) Release(_ context.Context, id uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.down {
		return errDown
	}
	if l.holds[id] == "ACTIVE" {
		l.holds[id] = "RELEASED"
	}
	return nil
}

type fakeAccounts struct {
	mu       sync.Mutex
	usage    map[string]int64 // txID -> amount (restored ones removed)
	limit    int64
	inactive map[uuid.UUID]bool
	restores int
}

func (a *fakeAccounts) ConsumeLimit(_ context.Context, account uuid.UUID, txID string, amount money.Money) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inactive[account] {
		return domain.Reject("ACCOUNT_NOT_ACTIVE", "blocked")
	}
	if _, ok := a.usage[txID]; ok {
		return nil
	}
	var used int64
	for _, v := range a.usage {
		used += v
	}
	if used+amount.Minor() > a.limit {
		return domain.Reject("LIMIT_EXCEEDED", "daily limit")
	}
	a.usage[txID] = amount.Minor()
	return nil
}

func (a *fakeAccounts) RestoreLimit(_ context.Context, _ uuid.UUID, txID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.usage, txID)
	a.restores++
	return nil
}

func (a *fakeAccounts) EnsureActive(_ context.Context, account uuid.UUID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inactive[account] {
		return domain.Reject("DESTINATION_NOT_ACTIVE", "inactive")
	}
	return nil
}

type env struct {
	svc      *application.Service
	ledger   *fakeLedger
	accounts *fakeAccounts
	pool     *pgxpool.Pool
	alice    uuid.UUID
	bob      uuid.UUID
	pix      uuid.UUID
}

func setup(t *testing.T) env {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	l := newLedger()
	a := &fakeAccounts{usage: map[string]int64{}, limit: 100_000, inactive: map[uuid.UUID]bool{}}
	e := env{
		svc:    application.New(pgstore.NewStore(pool), l, a, slog.New(slog.NewTextHandler(io.Discard, nil))),
		ledger: l, accounts: a, pool: pool,
		alice: uuid.New(), bob: uuid.New(), pix: uuid.New(),
	}
	l.balances[e.alice] = 50_000
	return e
}

func brl(s string) money.Money { return money.MustParse(s, money.BRL) }

func steps(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT step || ':' || outcome FROM saga_steps WHERE transaction_id = $1 ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

func events(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) []string {
	t.Helper()
	rows, _ := pool.Query(context.Background(), `SELECT payload->>'type' FROM outbox WHERE key = $1 ORDER BY created_at`, id.String())
	var out []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

func TestTransferCompletes(t *testing.T) {
	e := setup(t)
	tx, err := e.svc.Transfer(context.Background(), "k1", e.alice, e.bob, brl("100"), "rent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if tx.Status != domain.StatusCompleted || tx.EntryID == uuid.Nil {
		t.Fatalf("tx = %+v", tx)
	}
	if e.ledger.balances[e.bob] != 10_000 {
		t.Fatalf("bob = %d", e.ledger.balances[e.bob])
	}
	if got := events(t, e.pool, tx.ID); len(got) != 1 || got[0] != "transaction.completed" {
		t.Fatalf("events = %v", got)
	}
	again, err := e.svc.Transfer(context.Background(), "k1", e.alice, e.bob, brl("100"), "rent", nil)
	if err != nil || again.ID != tx.ID || e.ledger.balances[e.bob] != 10_000 {
		t.Fatalf("retry moved money again or failed: %v", err)
	}
	if _, err := e.svc.Transfer(context.Background(), "k1", e.alice, e.bob, brl("101"), "rent", nil); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("conflict err = %v", err)
	}
}

func TestTransferInsufficientFundsCompensatesLimit(t *testing.T) {
	e := setup(t)
	tx, err := e.svc.Transfer(context.Background(), "k", e.alice, e.bob, brl("600"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if tx.Status != domain.StatusFailed || tx.FailureReason != "INSUFFICIENT_FUNDS" {
		t.Fatalf("tx = %+v", tx)
	}
	if len(e.accounts.usage) != 0 || e.accounts.restores != 1 {
		t.Fatalf("limit not restored: usage=%v restores=%d", e.accounts.usage, e.accounts.restores)
	}
	want := []string{"consume_limit:OK", "post_entry:REJECTED", "restore_limit:OK", "fail:OK"}
	if got := steps(t, e.pool, tx.ID); len(got) != len(want) || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	if got := events(t, e.pool, tx.ID); len(got) != 1 || got[0] != "transaction.failed" {
		t.Fatalf("events = %v", got)
	}
}

func TestTransferRejectedBeforeMovingMoney(t *testing.T) {
	e := setup(t)
	e.accounts.limit = 1_000
	tx, _ := e.svc.Transfer(context.Background(), "k1", e.alice, e.bob, brl("20"), "", nil)
	if tx.Status != domain.StatusFailed || tx.FailureReason != "LIMIT_EXCEEDED" || e.accounts.restores != 0 {
		t.Fatalf("limit: %+v restores=%d", tx, e.accounts.restores)
	}
	e.accounts.inactive[e.bob] = true
	tx, _ = e.svc.Transfer(context.Background(), "k2", e.alice, e.bob, brl("1"), "", nil)
	if tx.Status != domain.StatusFailed || tx.FailureReason != "DESTINATION_NOT_ACTIVE" {
		t.Fatalf("inactive destination: %+v", tx)
	}
}

func TestTechnicalFailureIsRecovered(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// The ledger posts the entry but the response is lost.
	e.ledger.lostResponse = true
	tx, err := e.svc.Transfer(ctx, "k", e.alice, e.bob, brl("10"), "", nil)
	if err == nil {
		t.Fatal("expected a technical error")
	}
	if tx.Status != domain.StatusPending || !tx.LimitConsumed {
		t.Fatalf("saga should stay PENDING with the limit consumed: %+v", tx)
	}

	// Compensating now would be wrong (the money did move), so it is refused.
	if _, err := e.svc.Compensate(ctx, tx.ID, ""); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("compensating PENDING err = %v", err)
	}

	// The recovery worker replays the step; the ledger returns the same entry.
	if n, err := e.svc.RecoverOnce(ctx, 0, time.Hour); err != nil || n != 1 {
		t.Fatalf("recover = %d, %v", n, err)
	}
	got, _ := e.svc.Get(ctx, tx.ID)
	if got.Status != domain.StatusCompleted || e.ledger.balances[e.bob] != 1_000 {
		t.Fatalf("after recovery: %+v bob=%d", got, e.ledger.balances[e.bob])
	}
}

func TestDownstreamDownThenClientRetry(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.ledger.down = true
	if _, err := e.svc.Transfer(ctx, "k", e.alice, e.bob, brl("10"), "", nil); err == nil {
		t.Fatal("expected error while ledger is down")
	}
	e.ledger.down = false
	tx, err := e.svc.Transfer(ctx, "k", e.alice, e.bob, brl("10"), "", nil)
	if err != nil || tx.Status != domain.StatusCompleted {
		t.Fatalf("retry = %+v, %v", tx, err)
	}
	if len(e.accounts.usage) != 1 {
		t.Fatalf("limit consumed %d times", len(e.accounts.usage))
	}
}

func TestPixOutReserveComplete(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tx, err := e.svc.ReserveOutgoing(ctx, "pix-1", domain.TypePixOut, e.alice, e.pix, brl("30"), "pix", map[string]string{"end_to_end_id": "E1"})
	if err != nil || tx.Status != domain.StatusReserved || tx.HoldID == uuid.Nil {
		t.Fatalf("reserve = %+v, %v", tx, err)
	}
	done, err := e.svc.Complete(ctx, tx.ID)
	if err != nil || done.Status != domain.StatusCompleted || done.EntryID == uuid.Nil {
		t.Fatalf("complete = %+v, %v", done, err)
	}
	if again, err := e.svc.Complete(ctx, tx.ID); err != nil || again.Status != domain.StatusCompleted {
		t.Fatalf("complete retry = %+v, %v", again, err)
	}
	if _, err := e.svc.Compensate(ctx, tx.ID, ""); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("compensate after complete err = %v", err)
	}
	if got := events(t, e.pool, tx.ID); len(got) != 2 || got[0] != "transaction.reserved" || got[1] != "transaction.completed" {
		t.Fatalf("events = %v", got)
	}
}

func TestPixOutCompensate(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tx, _ := e.svc.ReserveOutgoing(ctx, "pix-1", domain.TypePixOut, e.alice, e.pix, brl("30"), "", nil)
	failed, err := e.svc.Compensate(ctx, tx.ID, "SPI_REJECTED")
	if err != nil || failed.Status != domain.StatusFailed || failed.FailureReason != "SPI_REJECTED" {
		t.Fatalf("compensate = %+v, %v", failed, err)
	}
	if e.ledger.holds[tx.HoldID] != "RELEASED" || len(e.accounts.usage) != 0 {
		t.Fatalf("hold=%s usage=%v", e.ledger.holds[tx.HoldID], e.accounts.usage)
	}
	if _, err := e.svc.Complete(ctx, tx.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("complete after compensate err = %v", err)
	}
}

func TestReservationTimeout(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tx, _ := e.svc.ReserveOutgoing(ctx, "pix-1", domain.TypePixOut, e.alice, e.pix, brl("30"), "", nil)
	if _, err := e.svc.RecoverOnce(ctx, time.Hour, 0); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, tx.ID)
	if got.Status != domain.StatusFailed || got.FailureReason != "RESERVATION_TIMEOUT" {
		t.Fatalf("after timeout: %+v", got)
	}
}

func TestPixInCredits(t *testing.T) {
	e := setup(t)
	e.ledger.balances[e.pix] = 1_000_000 // the fake has no negative internal balances
	tx, err := e.svc.CreditIncoming(context.Background(), "in-1", domain.TypePixIn, e.pix, e.bob, brl("5"), "pix in", nil)
	if err != nil || tx.Status != domain.StatusCompleted || e.ledger.balances[e.bob] != 500 {
		t.Fatalf("tx = %+v, %v", tx, err)
	}
	if len(e.accounts.usage) != 0 {
		t.Fatal("incoming money must not consume limits")
	}
	e.accounts.inactive[e.bob] = true
	tx, _ = e.svc.CreditIncoming(context.Background(), "in-2", domain.TypePixIn, e.pix, e.bob, brl("5"), "pix in", nil)
	if tx.Status != domain.StatusFailed || tx.FailureReason != "DESTINATION_NOT_ACTIVE" {
		t.Fatalf("credit to inactive account: %+v", tx)
	}
}

func TestConcurrentRunsAreSerialized(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.ledger.down = true
	tx, _ := e.svc.Transfer(ctx, "k", e.alice, e.bob, brl("10"), "", nil)
	e.ledger.down = false

	var wg sync.WaitGroup
	results := make(chan error, 10)
	for range 10 {
		wg.Go(func() {
			_, err := e.svc.Resume(ctx, tx.ID)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, domain.ErrBusy) {
			t.Errorf("unexpected: %v", err)
		}
	}
	got, _ := e.svc.Get(ctx, tx.ID)
	if got.Status != domain.StatusCompleted || len(e.ledger.entries) != 1 {
		t.Fatalf("status=%s entries=%d", got.Status, len(e.ledger.entries))
	}
}
