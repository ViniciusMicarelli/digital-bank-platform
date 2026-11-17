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
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/ports"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/migrations"
)

type fakeAccounts struct{ active map[uuid.UUID]bool }

func (f fakeAccounts) Active(_ context.Context, id uuid.UUID) (bool, error) {
	a, ok := f.active[id]
	if !ok {
		return false, domain.ErrNotFound
	}
	return a, nil
}

// fakeLedger posts idempotently; customer accounts cannot go negative.
type fakeLedger struct {
	mu        sync.Mutex
	balance   map[uuid.UUID]int64
	byKey     map[string]ports.TxResult
	down      bool
	lostReply bool
}

func (f *fakeLedger) Post(_ context.Context, key, _ string, from, to uuid.UUID, amount int64, _ string, _ map[string]string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return ports.TxResult{}, errors.New("transaction-service unavailable")
	}
	if r, ok := f.byKey[key]; ok {
		return r, nil
	}
	r := ports.TxResult{ID: uuid.New(), Status: "COMPLETED"}
	internal := from == application.CreditReceivablesAccountID
	if !internal && f.balance[from] < amount {
		r.Status, r.FailureReason = "FAILED", "INSUFFICIENT_FUNDS"
	} else {
		f.balance[from] -= amount
		f.balance[to] += amount
	}
	f.byKey[key] = r
	if f.lostReply {
		f.lostReply = false
		return ports.TxResult{}, errors.New("deadline exceeded")
	}
	return r, nil
}

type env struct {
	ctx     context.Context
	svc     *application.Service
	ledger  *fakeLedger
	account uuid.UUID
	now     time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	e := &env{ctx: context.Background(), account: uuid.New(),
		ledger: &fakeLedger{balance: map[uuid.UUID]int64{}, byKey: map[string]ports.TxResult{}},
		now:    time.Date(2026, 11, 17, 15, 0, 0, 0, time.UTC)}
	e.svc = application.New(pgstore.NewStore(pool), fakeAccounts{active: map[uuid.UUID]bool{e.account: true}},
		e.ledger, slog.New(slog.NewTextHandler(io.Discard, nil)), 1_000_00)
	e.svc.SetClock(func() time.Time { return e.now })
	return e
}

func (e *env) line(t *testing.T) domain.Line {
	t.Helper()
	l, err := e.svc.Line(e.ctx, e.account)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func (e *env) invoices(t *testing.T) []domain.Invoice {
	t.Helper()
	list, err := e.svc.Invoices(e.ctx, e.account)
	if err != nil {
		t.Fatal(err)
	}
	return list // newest first
}

// checkLedger: what customers owe (used, without holds) is exactly minus the
// credit-receivables balance.
func (e *env) checkLedger(t *testing.T, held int64) {
	t.Helper()
	if owed := e.line(t).Used - held; -e.ledger.balance[application.CreditReceivablesAccountID] != owed {
		t.Fatalf("receivables %d, owed %d", e.ledger.balance[application.CreditReceivablesAccountID], owed)
	}
}

func TestOpenLine(t *testing.T) {
	e := setup(t)
	l, err := e.svc.OpenLine(e.ctx, "k1", e.account, 10)
	if err != nil || l.Limit != 1_000_00 || l.Status != domain.LineActive {
		t.Fatalf("open = %+v %v", l, err)
	}
	if again, err := e.svc.OpenLine(e.ctx, "k1", e.account, 10); err != nil || again.ID != l.ID {
		t.Fatalf("same key = %+v %v", again, err)
	}
	var refused *domain.RefusedError
	if _, err := e.svc.OpenLine(e.ctx, "k2", e.account, 10); !errors.As(err, &refused) || refused.Reason != "CREDIT_LINE_EXISTS" {
		t.Fatalf("second line: %v", err)
	}
	if _, err := e.svc.OpenLine(e.ctx, "k3", uuid.New(), 10); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
	inv := e.invoices(t)
	if len(inv) != 1 || inv[0].Status != domain.InvoiceOpen || inv[0].ClosingDate.Format(time.DateOnly) != "2026-12-10" {
		t.Fatalf("first invoice = %+v", inv)
	}
}

func TestHolds(t *testing.T) {
	e := setup(t)
	if h, _ := e.svc.Reserve(e.ctx, "h0", e.account, 10_00, "LOJA", nil); h.FailureReason != domain.ReasonNoCreditLine {
		t.Fatalf("no line: %+v", h)
	}
	_, _ = e.svc.OpenLine(e.ctx, "k", e.account, 10)
	a, err := e.svc.Reserve(e.ctx, "h1", e.account, 600_00, "TV", nil)
	if err != nil || a.Status != domain.HoldReserved {
		t.Fatalf("reserve = %+v %v", a, err)
	}
	if again, _ := e.svc.Reserve(e.ctx, "h1", e.account, 600_00, "TV", nil); again.ID != a.ID || e.line(t).Used != 600_00 {
		t.Fatal("a retried reservation must not hold twice")
	}
	if _, err := e.svc.Reserve(e.ctx, "h1", e.account, 1_00, "TV", nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("same key, other amount: %v", err)
	}
	if b, _ := e.svc.Reserve(e.ctx, "h2", e.account, 400_01, "SOFA", nil); b.FailureReason != domain.ReasonInsufficientLimit {
		t.Fatalf("over the limit: %+v", b)
	}
	b, _ := e.svc.Reserve(e.ctx, "h3", e.account, 100_00, "SOFA", nil)
	if _, err := e.svc.Release(e.ctx, b.ID); err != nil || e.line(t).Used != 600_00 {
		t.Fatalf("release: %v, used %d", err, e.line(t).Used)
	}

	partial := int64(550_00)
	c, err := e.svc.Capture(e.ctx, a.ID, &partial)
	if err != nil || c.Status != domain.HoldCaptured || c.Captured != 550_00 {
		t.Fatalf("capture = %+v %v", c, err)
	}
	if _, err := e.svc.Capture(e.ctx, a.ID, &partial); err != nil {
		t.Fatal("capturing again is a no-op")
	}
	if _, err := e.svc.Release(e.ctx, a.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatal("a captured purchase cannot be released")
	}
	if inv := e.invoices(t)[0]; inv.Total != 550_00 || e.line(t).Used != 550_00 {
		t.Fatalf("invoice %+v, used %d", inv, e.line(t).Used)
	}
	e.checkLedger(t, 0)

	// Holds the network never clears expire after 7 days.
	d, _ := e.svc.Reserve(e.ctx, "h4", e.account, 50_00, "HOTEL", nil)
	e.now = e.now.Add(domain.HoldLifetime + time.Hour)
	e.svc.RunOnce(e.ctx)
	if h, _ := e.svc.Reserve(e.ctx, "h4", e.account, 50_00, "HOTEL", nil); h.ID != d.ID || h.Status != domain.HoldReleased {
		t.Fatalf("expired hold = %+v", h)
	}
}

func TestInvoiceCycle(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.OpenLine(e.ctx, "k", e.account, 10)
	e.ledger.balance[e.account] = 300_00
	h, _ := e.svc.Reserve(e.ctx, "h1", e.account, 500_00, "TV", nil)
	if _, err := e.svc.Capture(e.ctx, h.ID, nil); err != nil {
		t.Fatal(err)
	}

	// Dec 10: the invoice closes and the next one opens.
	e.now = time.Date(2026, 12, 10, 12, 0, 0, 0, time.UTC)
	e.svc.RunOnce(e.ctx)
	list := e.invoices(t)
	if len(list) != 2 || list[1].Status != domain.InvoiceClosed || list[1].DueDate.Format(time.DateOnly) != "2026-12-20" ||
		list[0].Status != domain.InvoiceOpen || list[0].ClosingDate.Format(time.DateOnly) != "2027-01-10" {
		t.Fatalf("after closing = %+v", list)
	}
	closed := list[1]

	// Paying more than is owed, or more than the account has.
	var refused *domain.RefusedError
	if _, err := e.svc.Pay(e.ctx, "p0", closed.ID, 500_01); !errors.As(err, &refused) || refused.Reason != "PAYMENT_TOO_LARGE" {
		t.Fatalf("overpaying: %v", err)
	}
	if p, err := e.svc.Pay(e.ctx, "p1", closed.ID, 400_00); err != nil || p.Status != domain.PaymentFailed || p.FailureReason != "INSUFFICIENT_FUNDS" {
		t.Fatalf("no money = %+v %v", p, err)
	}
	// A payment whose reply is lost stays PENDING and the worker settles it.
	e.ledger.lostReply = true
	if _, err := e.svc.Pay(e.ctx, "p2", closed.ID, 200_00); err == nil {
		t.Fatal("lost reply should surface")
	}
	e.now = e.now.Add(time.Minute)
	e.svc.RunOnce(e.ctx)
	if p, err := e.svc.Pay(e.ctx, "p2", closed.ID, 200_00); err != nil || p.Status != domain.PaymentCompleted {
		t.Fatalf("settled payment = %+v %v", p, err)
	}
	if e.line(t).Used != 300_00 || e.ledger.balance[e.account] != 100_00 {
		t.Fatalf("used %d, balance %d", e.line(t).Used, e.ledger.balance[e.account])
	}
	e.checkLedger(t, 0)

	// Dec 21: 300.00 unpaid moves to the next invoice with 2% fine and 8% interest.
	e.now = time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)
	e.svc.RunOnce(e.ctx)
	list = e.invoices(t)
	if list[1].Status != domain.InvoiceOverdue || list[0].Total != 300_00+6_00+24_00 || e.line(t).Used != 330_00 {
		t.Fatalf("after due date = %+v, used %d", list, e.line(t).Used)
	}
	if e.ledger.balance[application.CreditRevenueAccountID] != 30_00 {
		t.Fatalf("revenue = %d", e.ledger.balance[application.CreditRevenueAccountID])
	}
	e.checkLedger(t, 0)
	if _, err := e.svc.Pay(e.ctx, "p3", closed.ID, 1_00); !errors.As(err, &refused) || refused.Reason != "INVOICE_NOT_PAYABLE" {
		t.Fatalf("paying an overdue invoice: %v", err)
	}
	d, err := e.svc.Invoice(e.ctx, list[0].ID)
	if err != nil || len(d.Items) != 3 || d.AccountID != e.account {
		t.Fatalf("next invoice items = %+v %v", d, err)
	}

	// Paying it all ahead of closing; the next close finds it paid.
	e.ledger.balance[e.account] = 1_000_00
	if p, err := e.svc.Pay(e.ctx, "p4", list[0].ID, 330_00); err != nil || p.Status != domain.PaymentCompleted {
		t.Fatalf("advance payment = %+v %v", p, err)
	}
	if err := e.svc.AdvanceCycle(e.ctx, e.account, "CLOSE"); err != nil {
		t.Fatal(err)
	}
	if list := e.invoices(t); list[1].Status != domain.InvoicePaid || e.line(t).Used != 0 {
		t.Fatalf("paid in advance = %+v", list)
	}
	e.checkLedger(t, 0)
}

func TestSandboxExpire(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.OpenLine(e.ctx, "k", e.account, 10)
	h, _ := e.svc.Reserve(e.ctx, "h", e.account, 100_00, "CAFE", nil)
	_, _ = e.svc.Capture(e.ctx, h.ID, nil)
	if err := e.svc.AdvanceCycle(e.ctx, e.account, "CLOSE"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.AdvanceCycle(e.ctx, e.account, "EXPIRE"); err != nil {
		t.Fatal(err)
	}
	if list := e.invoices(t); list[1].Status != domain.InvoiceOverdue || list[0].Total != 110_00 {
		t.Fatalf("expired = %+v", list)
	}
	e.checkLedger(t, 0)
}
