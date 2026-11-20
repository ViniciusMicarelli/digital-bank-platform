package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestCycle(t *testing.T) {
	if got := FirstClosing(10, date(2026, 11, 17)); !got.Equal(date(2026, 12, 10)) {
		t.Fatalf("after the closing day: %v", got)
	}
	if got := FirstClosing(10, date(2026, 11, 10)); !got.Equal(date(2026, 12, 10)) {
		t.Fatalf("on the closing day: %v", got)
	}
	if got := FirstClosing(25, date(2026, 12, 17)); !got.Equal(date(2026, 12, 25)) {
		t.Fatalf("before the closing day: %v", got)
	}
	inv := NewInvoice(uuid.New(), date(2026, 12, 25))
	if !inv.DueDate.Equal(date(2027, 1, 4)) || !inv.Next().ClosingDate.Equal(date(2027, 1, 25)) {
		t.Fatalf("due %v, next %v", inv.DueDate, inv.Next().ClosingDate)
	}
	// 23:30 on Nov 17 in São Paulo is already Nov 18 in UTC.
	if got := Today(time.Date(2026, 11, 18, 2, 30, 0, 0, time.UTC)); !got.Equal(date(2026, 11, 17)) {
		t.Fatalf("today = %v", got)
	}
}

func TestLine(t *testing.T) {
	if _, err := NewLine("k", uuid.New(), 29, DefaultLimit); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("closing day 29")
	}
	l, _ := NewLine("k", uuid.New(), 10, 100_00)
	if r := l.Reserve(60_00); r != "" || l.Available() != 40_00 {
		t.Fatalf("reserve: %q, available %d", r, l.Available())
	}
	if r := l.Reserve(40_01); r != ReasonInsufficientLimit {
		t.Fatalf("over the limit: %q", r)
	}
	l.Free(10_00)
	l.Status = LineBlocked
	if r := l.Reserve(1); r != ReasonLineBlocked || l.Used != 50_00 {
		t.Fatalf("blocked: %q used %d", r, l.Used)
	}
}

func TestInvoicePayments(t *testing.T) {
	inv := NewInvoice(uuid.New(), date(2026, 12, 10))
	inv.Total = 300_00
	if err := inv.StartPayment(100_00); err != nil {
		t.Fatal(err) // paying ahead of closing is allowed
	}
	var refused *RefusedError
	if err := inv.StartPayment(200_01); !errors.As(err, &refused) || refused.Reason != "PAYMENT_TOO_LARGE" {
		t.Fatalf("payment in flight counts: %v", err)
	}
	inv.SettlePayment(100_00, true)
	_ = inv.Close()
	if inv.Status != InvoiceClosed || inv.Remaining() != 200_00 {
		t.Fatalf("closed = %+v", inv)
	}
	if !inv.Overdue(date(2026, 12, 21)) || inv.Overdue(date(2026, 12, 20)) {
		t.Fatal("overdue the day after the due date")
	}
	_ = inv.StartPayment(200_00)
	inv.SettlePayment(200_00, false)
	if inv.Remaining() != 200_00 || inv.Pending != 0 {
		t.Fatalf("failed payment = %+v", inv)
	}
	_ = inv.StartPayment(200_00)
	inv.SettlePayment(200_00, true)
	if inv.Status != InvoicePaid {
		t.Fatalf("fully paid = %+v", inv)
	}
	empty := NewInvoice(uuid.New(), date(2026, 12, 10))
	_ = empty.Close()
	if empty.Status != InvoicePaid {
		t.Fatal("an empty invoice closes paid")
	}
}

func TestCharges(t *testing.T) {
	fine, interest := Charges(200_00, DefaultInterestRateBP)
	if fine != 4_00 || interest != 16_00 {
		t.Fatalf("fine %d interest %d", fine, interest)
	}
	if fine, _ := Charges(25, DefaultInterestRateBP); fine != 1 {
		t.Fatalf("rounding: %d", fine)
	}
}

func TestComputeScore(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	line := Line{Limit: 1_000_00, CreatedAt: now}
	if s := ComputeScore(ScoreInputs{Line: line}, now); s.Value != 580 || s.Band != BandGood || len(s.Factors) != 6 {
		t.Fatalf("a new line = %+v", s)
	}
	// Two years, eight invoices on time, 20% used.
	good := ScoreInputs{Line: Line{Limit: 1_000_00, Used: 200_00, CreatedAt: now.AddDate(-2, 0, 0)}, OnTime: 8}
	if s := ComputeScore(good, now); s.Value != 500+240+80+120 || s.Band != BandExcellent ||
		s.Factor(FactorRelationship).Value != 24 || s.Factor(FactorUtilization).Value != 20 {
		t.Fatalf("a good customer = %+v", s)
	}
	// Late twice, overdue now, maxed out, one plan: clamped at 0.
	bad := ScoreInputs{Line: Line{Limit: 1_000_00, Used: 1_000_00, CreatedAt: now}, Late: 2, OverdueNow: true, Plans: 1}
	if s := ComputeScore(bad, now); s.Value != 0 || s.Band != BandLow || s.Factor(FactorLatePayments).Points != -240 {
		t.Fatalf("a bad customer = %+v", s)
	}
	if MonthsBetween(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)) != 1 {
		t.Fatal("full months only")
	}
}

func TestAutoIncreaseBy(t *testing.T) {
	for score, want := range map[int]int{800: 10_000, 600: 5_000, 300: 0} {
		if got := AutoIncreaseBy(score); got != want {
			t.Errorf("AutoIncreaseBy(%d) = %d, want %d", score, got, want)
		}
	}
}

func TestAnalyzeLimitByScore(t *testing.T) {
	l := Line{Status: LineActive, Limit: 1_000_00}
	for _, c := range []struct {
		requested int64
		score     int
		status    LimitStatus
		reason    string
	}{
		{1_500_00, 600, LimitApproved, ""},
		{1_600_00, 600, LimitInReview, ""},
		{2_000_00, 800, LimitApproved, ""}, // +100% from 750
		{1_100_00, 400, LimitInReview, ""}, // no automatic increase under 450
		{1_100_00, 200, LimitRejected, "LOW_SCORE"},
		{500_00, 0, LimitApproved, ""}, // a decrease never depends on the score
	} {
		status, reason, err := AnalyzeLimit(l, c.requested, false, c.score)
		if err != nil || status != c.status || reason != c.reason {
			t.Errorf("AnalyzeLimit(%d, score %d) = %s %q %v, want %s %q", c.requested, c.score, status, reason, err, c.status, c.reason)
		}
	}
}
