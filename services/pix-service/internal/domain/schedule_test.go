package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestMonthlyNeverDrifts(t *testing.T) {
	s, err := NewSchedule("k", uuid.New(), "x@y.com", money.MustParse("10", money.BRL), "", date("2027-01-31"), Monthly, 0, date("2026-12-01"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2027-01-31", "2027-02-28", "2027-03-31", "2027-04-30", "2027-05-31"}
	for i, w := range want {
		if got := s.OccurrenceDate(i).Format("2006-01-02"); got != w {
			t.Errorf("occurrence %d = %s, want %s", i, got, w)
		}
	}
	if leap := s.OccurrenceDate(13).Format("2006-01-02"); leap != "2028-02-29" {
		t.Errorf("leap year February = %s", leap)
	}
}

func TestScheduleLifecycle(t *testing.T) {
	today := date("2026-11-17")
	s, _ := NewSchedule("k", uuid.New(), "x@y.com", money.MustParse("10", money.BRL), "", date("2026-11-20"), Weekly, 3, today)
	if s.Due(today) || !s.Due(date("2026-11-20")) {
		t.Fatal("due dates")
	}
	s.Advance()
	if s.NextDate != date("2026-11-27") || s.Status != ScheduleActive {
		t.Fatalf("after 1: %+v", s)
	}
	s.Advance()
	s.Advance()
	if s.Status != ScheduleCompleted || s.Executed != 3 {
		t.Fatalf("after 3: %+v", s)
	}
	if err := s.Cancel(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("a completed schedule cannot be cancelled")
	}
	if _, err := NewSchedule("k", uuid.New(), "x", money.MustParse("1", money.BRL), "", date("2026-11-16"), Once, 0, today); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("past date accepted")
	}
	if s.TransferKey(2) != "sched:"+s.ID.String()+":2" {
		t.Fatal(s.TransferKey(2))
	}
}

func TestTodayUsesSaoPaulo(t *testing.T) {
	// 01:30 UTC on the 18th is still the 17th in São Paulo (UTC-3).
	if got := Today(time.Date(2026, 11, 18, 1, 30, 0, 0, time.UTC)); got != date("2026-11-17") {
		t.Fatalf("today = %s", got)
	}
}
