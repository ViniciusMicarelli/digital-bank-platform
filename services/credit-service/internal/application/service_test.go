package application_test

import (
	"context"
	"errors"
	"fmt"
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
	internal := from == application.CreditReceivablesAccountID || from == application.CardClearingAccountID
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
	if h, _ := e.svc.Reserve(e.ctx, "h0", e.account, 10_00, 1, "LOJA", nil); h.FailureReason != domain.ReasonNoCreditLine {
		t.Fatalf("no line: %+v", h)
	}
	_, _ = e.svc.OpenLine(e.ctx, "k", e.account, 10)
	a, err := e.svc.Reserve(e.ctx, "h1", e.account, 600_00, 1, "TV", nil)
	if err != nil || a.Status != domain.HoldReserved {
		t.Fatalf("reserve = %+v %v", a, err)
	}
	if again, _ := e.svc.Reserve(e.ctx, "h1", e.account, 600_00, 1, "TV", nil); again.ID != a.ID || e.line(t).Used != 600_00 {
		t.Fatal("a retried reservation must not hold twice")
	}
	if _, err := e.svc.Reserve(e.ctx, "h1", e.account, 1_00, 1, "TV", nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("same key, other amount: %v", err)
	}
	if b, _ := e.svc.Reserve(e.ctx, "h2", e.account, 400_01, 1, "SOFA", nil); b.FailureReason != domain.ReasonInsufficientLimit {
		t.Fatalf("over the limit: %+v", b)
	}
	b, _ := e.svc.Reserve(e.ctx, "h3", e.account, 100_00, 1, "SOFA", nil)
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
	d, _ := e.svc.Reserve(e.ctx, "h4", e.account, 50_00, 1, "HOTEL", nil)
	e.now = e.now.Add(domain.HoldLifetime + time.Hour)
	e.svc.RunOnce(e.ctx)
	if h, _ := e.svc.Reserve(e.ctx, "h4", e.account, 50_00, 1, "HOTEL", nil); h.ID != d.ID || h.Status != domain.HoldReleased {
		t.Fatalf("expired hold = %+v", h)
	}
}

func TestInvoiceCycle(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.OpenLine(e.ctx, "k", e.account, 10)
	e.ledger.balance[e.account] = 300_00
	h, _ := e.svc.Reserve(e.ctx, "h1", e.account, 500_00, 1, "TV", nil)
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
	h, _ := e.svc.Reserve(e.ctx, "h", e.account, 100_00, 1, "CAFE", nil)
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

func TestRefunds(t *testing.T) {
	e := setup(t)
	if _, err := e.svc.Refund(e.ctx, "r0", e.account, 10_00, "ESTORNO", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no line: %v", err)
	}
	_, _ = e.svc.OpenLine(e.ctx, "k", e.account, 10)
	h, _ := e.svc.Reserve(e.ctx, "h1", e.account, 100_00, 1, "LOJA", nil)
	if _, err := e.svc.Capture(e.ctx, h.ID, nil); err != nil {
		t.Fatal(err)
	}

	r, err := e.svc.Refund(e.ctx, "r1", e.account, 30_00, "Estorno LOJA", nil)
	if err != nil || r.Amount != 30_00 {
		t.Fatalf("refund = %+v %v", r, err)
	}
	if again, err := e.svc.Refund(e.ctx, "r1", e.account, 30_00, "Estorno LOJA", nil); err != nil || again.ID != r.ID {
		t.Fatalf("retry = %+v %v", again, err)
	}
	if _, err := e.svc.Refund(e.ctx, "r1", e.account, 31_00, "x", nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("same key, other amount: %v", err)
	}
	if inv := e.invoices(t)[0]; inv.Total != 70_00 || e.line(t).Used != 70_00 {
		t.Fatalf("after a refund: invoice %+v, used %d", inv, e.line(t).Used)
	}
	e.checkLedger(t, 0)

	// A lost reply from the ledger: the retry does not credit twice.
	e.ledger.lostReply = true
	if _, err := e.svc.Refund(e.ctx, "r2", e.account, 10_00, "Estorno", nil); err == nil {
		t.Fatal("expected the lost reply")
	}
	if _, err := e.svc.Refund(e.ctx, "r2", e.account, 10_00, "Estorno", nil); err != nil {
		t.Fatal(err)
	}
	if inv := e.invoices(t)[0]; inv.Total != 60_00 {
		t.Fatalf("after the retried refund: %+v", inv)
	}
	e.checkLedger(t, 0)

	// More refunded than owed: the credit moves to the next invoice.
	if _, err := e.svc.Refund(e.ctx, "r3", e.account, 80_00, "Estorno", nil); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.AdvanceCycle(e.ctx, e.account, "CLOSE"); err != nil {
		t.Fatal(err)
	}
	list := e.invoices(t)
	if len(list) != 2 || list[1].Status != domain.InvoicePaid || list[1].Total != 0 || list[0].Total != -20_00 {
		t.Fatalf("after closing = %+v", list)
	}
	d, _ := e.svc.Invoice(e.ctx, list[0].ID)
	if len(d.Items) != 1 || d.Items[0].Kind != domain.ItemCreditCarriedIn || d.Items[0].Amount != 20_00 {
		t.Fatalf("next invoice items = %+v", d.Items)
	}
}

func TestInstallments(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.OpenLine(e.ctx, "k", e.account, 10)
	if _, err := e.svc.Reserve(e.ctx, "small", e.account, 9_00, 2, "X", nil); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("installments under R$ 5: %v", err)
	}
	if _, err := e.svc.Reserve(e.ctx, "many", e.account, 900_00, 13, "X", nil); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("13 installments: %v", err)
	}
	h, err := e.svc.Reserve(e.ctx, "tv", e.account, 900_01, 3, "LOJA DE TV", nil)
	if err != nil || h.Installments != 3 || e.line(t).Used != 900_01 {
		t.Fatalf("reserve = %+v %v", h, err)
	}
	if _, err := e.svc.Reserve(e.ctx, "tv", e.account, 900_01, 1, "LOJA DE TV", nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("same key, other installments")
	}
	if _, err := e.svc.Capture(e.ctx, h.ID, nil); err != nil {
		t.Fatal(err)
	}
	// The whole debt is on the limit; only the first installment (with the
	// remainder centavo) is on this invoice.
	inv := e.invoices(t)[0]
	if inv.Total != 300_01 || e.line(t).Used != 900_01 {
		t.Fatalf("invoice %+v, used %d", inv, e.line(t).Used)
	}
	if up, _ := e.svc.UpcomingInstallments(e.ctx, e.line(t)); up != 600_00 {
		t.Fatalf("upcoming = %d", up)
	}
	e.checkLedger(t, 0)
	d, _ := e.svc.Invoice(e.ctx, inv.ID)
	if len(d.Items) != 1 || d.Items[0].Description != "LOJA DE TV (1/3)" {
		t.Fatalf("items = %+v", d.Items)
	}

	// Each new invoice gets the next installment.
	for n := 2; n <= 3; n++ {
		if err := e.svc.AdvanceCycle(e.ctx, e.account, "CLOSE"); err != nil {
			t.Fatal(err)
		}
		open := e.invoices(t)[0]
		d, _ := e.svc.Invoice(e.ctx, open.ID)
		if open.Total != 300_00 || len(d.Items) != 1 || d.Items[0].Description != fmt.Sprintf("LOJA DE TV (%d/3)", n) {
			t.Fatalf("invoice %d = %+v %+v", n, open, d.Items)
		}
		plans, _ := e.svc.InstallmentPlans(e.ctx, e.account)
		if n < 3 && (len(plans) != 1 || plans[0].Billed != n || plans[0].Remaining != 300_00) {
			t.Fatalf("plans after %d = %+v", n, plans)
		}
		if n == 3 && len(plans) != 0 {
			t.Fatalf("a fully billed purchase has no plan: %+v", plans)
		}
	}
	if err := e.svc.AdvanceCycle(e.ctx, e.account, "CLOSE"); err != nil {
		t.Fatal(err)
	}
	if open := e.invoices(t)[0]; open.Total != 0 {
		t.Fatalf("nothing more to bill: %+v", open)
	}
}

func TestInvoicePlan(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.OpenLine(e.ctx, "k", e.account, 10)
	h, _ := e.svc.Reserve(e.ctx, "tv", e.account, 600_00, 1, "LOJA DE TV", nil)
	_, _ = e.svc.Capture(e.ctx, h.ID, nil)
	open := e.invoices(t)[0]
	if _, err := e.svc.PlanQuotes(e.ctx, open.ID); reason(err) != "INVOICE_NOT_PLANNABLE" {
		t.Fatalf("an open invoice cannot be split: %v", err)
	}
	if err := e.svc.AdvanceCycle(e.ctx, e.account, "CLOSE"); err != nil {
		t.Fatal(err)
	}
	closed := e.invoices(t)[1]
	if closed.Status != domain.InvoiceClosed || domain.MinimumPayment(closed.Remaining()) != 90_00 {
		t.Fatalf("closed = %+v, minimum %d", closed, domain.MinimumPayment(closed.Remaining()))
	}
	quotes, err := e.svc.PlanQuotes(e.ctx, closed.ID)
	if err != nil || len(quotes) != 11 || quotes[4].Installments != 6 || quotes[4].Interest <= 0 || quotes[4].AnnualCostBP < 7_000 {
		t.Fatalf("quotes = %+v %v", quotes, err)
	}
	q := quotes[4]
	plan, err := e.svc.PlanInvoice(e.ctx, "p1", closed.ID, 6)
	if err != nil || plan.Installment != q.Installment || plan.Interest() != q.Interest {
		t.Fatalf("plan = %+v %v", plan, err)
	}
	if again, _ := e.svc.PlanInvoice(e.ctx, "p1", closed.ID, 6); again.ID != plan.ID {
		t.Fatal("same key, same plan")
	}
	if _, err := e.svc.PlanInvoice(e.ctx, "p2", closed.ID, 3); reason(err) != "INVOICE_NOT_PLANNABLE" {
		t.Fatalf("a settled invoice: %v", err)
	}
	list := e.invoices(t)
	if list[1].Status != domain.InvoicePaid || list[0].Total != q.Installment {
		t.Fatalf("after the plan: closed %+v, open %+v", list[1], list[0])
	}
	if e.line(t).Used != 600_00+q.Interest {
		t.Fatalf("used = %d", e.line(t).Used)
	}
	e.checkLedger(t, 0) // the interest is in the ledger too

	// The next invoice gets 2/6.
	if err := e.svc.AdvanceCycle(e.ctx, e.account, "CLOSE"); err != nil {
		t.Fatal(err)
	}
	d, _ := e.svc.Invoice(e.ctx, e.invoices(t)[0].ID)
	if len(d.Items) != 1 || d.Items[0].Description != "Parcelamento da fatura (2/6)" {
		t.Fatalf("next invoice = %+v", d.Items)
	}
	plans, _ := e.svc.InstallmentPlans(e.ctx, e.account)
	if len(plans) != 1 || plans[0].Billed != 2 || plans[0].Remaining != 4*q.Installment {
		t.Fatalf("plans = %+v", plans)
	}
}

func TestMinimumPayment(t *testing.T) {
	for remaining, want := range map[int64]int64{0: 0, 30_00: 30_00, 200_00: 50_00, 1_000_00: 150_00} {
		if got := domain.MinimumPayment(remaining); got != want {
			t.Errorf("MinimumPayment(%d) = %d, want %d", remaining, got, want)
		}
	}
}

func reason(err error) string {
	var r *domain.RefusedError
	if errors.As(err, &r) {
		return r.Reason
	}
	return ""
}

func TestLimitRequests(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.OpenLine(e.ctx, "k", e.account, 10) // R$ 1.000
	h, _ := e.svc.Reserve(e.ctx, "h", e.account, 400_00, 1, "LOJA", nil)
	_, _ = e.svc.Capture(e.ctx, h.ID, nil)

	if _, err := e.svc.RequestLimit(e.ctx, "r0", e.account, 300_00); reason(err) != "LIMIT_BELOW_USED" {
		t.Fatalf("below what is used: %v", err)
	}
	down, err := e.svc.RequestLimit(e.ctx, "r1", e.account, 800_00)
	if err != nil || down.Status != domain.LimitApproved || e.line(t).Limit != 800_00 {
		t.Fatalf("decrease = %+v %v", down, err)
	}
	// Up to +50% is automatic.
	up, _ := e.svc.RequestLimit(e.ctx, "r2", e.account, 1_200_00)
	if up.Status != domain.LimitApproved || up.DecidedBy != "automatic" || e.line(t).Limit != 1_200_00 {
		t.Fatalf("automatic increase = %+v", up)
	}
	if again, _ := e.svc.RequestLimit(e.ctx, "r2", e.account, 1_200_00); again.ID != up.ID {
		t.Fatal("same key, same request")
	}
	// More goes to an analyst; one request in review at a time.
	review, _ := e.svc.RequestLimit(e.ctx, "r3", e.account, 5_000_00)
	if review.Status != domain.LimitInReview || e.line(t).Limit != 1_200_00 {
		t.Fatalf("large increase = %+v", review)
	}
	if _, err := e.svc.RequestLimit(e.ctx, "r4", e.account, 6_000_00); reason(err) != "LIMIT_REQUEST_IN_REVIEW" {
		t.Fatalf("second request in review: %v", err)
	}
	if list, _ := e.svc.LimitRequests(e.ctx, uuid.Nil, domain.LimitInReview); len(list) != 1 || list[0].ID != review.ID {
		t.Fatalf("queue = %+v", list)
	}
	if _, err := e.svc.DecideLimit(e.ctx, review.ID, "analyst-1", true, 6_000_00, ""); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatal("cannot grant more than asked")
	}
	decided, err := e.svc.DecideLimit(e.ctx, review.ID, "analyst-1", true, 3_000_00, "Renda compatível com R$ 3 mil")
	if err != nil || decided.Granted != 3_000_00 || e.line(t).Limit != 3_000_00 {
		t.Fatalf("partial approval = %+v %v", decided, err)
	}
	if _, err := e.svc.DecideLimit(e.ctx, review.ID, "analyst-2", false, 0, "x"); reason(err) != "LIMIT_REQUEST_NOT_IN_REVIEW" {
		t.Fatalf("decided twice: %v", err)
	}

	// An overdue invoice: increases are refused.
	if err := e.svc.AdvanceCycle(e.ctx, e.account, "CLOSE"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.AdvanceCycle(e.ctx, e.account, "EXPIRE"); err != nil {
		t.Fatal(err)
	}
	if r, _ := e.svc.RequestLimit(e.ctx, "r5", e.account, 3_100_00); r.Status != domain.LimitRejected || r.Reason != "HAS_OVERDUE_INVOICE" {
		t.Fatalf("with an overdue invoice = %+v", r)
	}
	if _, err := e.svc.RequestLimit(e.ctx, "r6", e.account, 1_234_56); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatal("whole reais only")
	}
}
