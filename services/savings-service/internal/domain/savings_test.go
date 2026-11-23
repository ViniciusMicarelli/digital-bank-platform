package domain

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccrue(t *testing.T) {
	// 10.65% a year: a year of daily accruals on R$ 1.000 pays about R$ 106,50.
	var carry int64
	balance := int64(1_000_00)
	for range 365 {
		var a int64
		a, carry = Accrue(balance, 1065, 1, carry)
		balance += a
	}
	if got := balance - 1_000_00; math.Abs(float64(got-106_50)) > 2 {
		t.Fatalf("a year yields %d centavos", got)
	}
	// Small balances: fractions of a centavo are carried, not lost.
	var paid int64
	carry = 0
	for range 30 {
		var a int64
		a, carry = Accrue(10_00, 1065, 1, carry) // R$ 10,00
		paid += a
	}
	if want, _ := Accrue(10_00, 1065, 30, 0); paid < want-1 || paid > want+1 {
		t.Fatalf("30 daily accruals paid %d, 30 days at once %d", paid, want)
	}
	if a, c := Accrue(0, 1065, 5, 7); a != 0 || c != 7 {
		t.Fatal("an empty box yields nothing")
	}
}

func TestBoxRules(t *testing.T) {
	now := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC) // 23:00 of the 18th in Brasília
	if _, err := NewBox("k", uuid.New(), "  ", 0, now); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("a name is required")
	}
	b, err := NewBox("k", uuid.New(), " Viagem ", 5_000_00, now)
	if err != nil || b.Name != "Viagem" || b.LastAccrued.Day() != 18 {
		t.Fatalf("box = %+v %v", b, err)
	}
	if _, err := NewMovement(b, "m", Deposit, 0, now); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("positive amounts only")
	}
	b.Status = Closed
	if _, err := NewMovement(b, "m", Deposit, 10, now); err == nil {
		t.Fatal("a closed box takes no deposits")
	}
	if err := b.Update("x", 0); err == nil {
		t.Fatal("a closed box cannot change")
	}
	if !EndOfDay(b.LastAccrued).Equal(time.Date(2026, 11, 19, 3, 0, 0, 0, time.UTC)) {
		t.Fatal(EndOfDay(b.LastAccrued))
	}
}

func TestNextRun(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse(time.DateOnly, s); return v }
	for _, c := range []struct {
		day         int
		after, want string
	}{
		{5, "2026-11-04", "2026-11-05"},
		{5, "2026-11-05", "2026-12-05"},
		{28, "2026-12-30", "2027-01-28"},
		{1, "2026-12-31", "2027-01-01"},
	} {
		if got := NextRun(c.day, d(c.after)); !got.Equal(d(c.want)) {
			t.Errorf("NextRun(%d, %s) = %s, want %s", c.day, c.after, got.Format(time.DateOnly), c.want)
		}
	}
}

func TestRoundUp(t *testing.T) {
	for amount, want := range map[int64]int64{12_30: 70, 12_00: 0, 1: 99, 99: 1, 0: 0} {
		if got := RoundUp(amount); got != want {
			t.Errorf("RoundUp(%d) = %d, want %d", amount, got, want)
		}
	}
	if MovementSource(RoundUpKey("a1")) != "ROUND_UP" || MovementSource(ScheduleKey(time.Now())) != "SCHEDULE" || MovementSource("d1") != "" {
		t.Fatal("sources")
	}
}
