package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

// Location is the bank's calendar: schedules run on São Paulo dates.
var Location = mustLoad("America/Sao_Paulo")

func mustLoad(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// Today is the current date in São Paulo, at midnight UTC (a pure date).
func Today(now time.Time) time.Time {
	l := now.In(Location)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

type Frequency string

const (
	Once    Frequency = "ONCE"
	Weekly  Frequency = "WEEKLY"
	Monthly Frequency = "MONTHLY"
)

type ScheduleStatus string

const (
	ScheduleActive    ScheduleStatus = "ACTIVE"
	ScheduleCancelled ScheduleStatus = "CANCELLED"
	ScheduleCompleted ScheduleStatus = "COMPLETED"
)

// Schedule is a future (possibly recurring) Pix from one of our accounts.
type Schedule struct {
	ID             uuid.UUID
	IdempotencyKey string
	AccountID      uuid.UUID
	Key            string
	Amount         money.Money
	Description    string
	StartDate      time.Time // first execution date
	Frequency      Frequency
	Occurrences    int // 0 = until cancelled (recurring only)
	Executed       int // occurrences already run
	NextDate       time.Time
	Status         ScheduleStatus
	MandateID      uuid.UUID // set when created by a Pix Automático charge
	CreatedAt      time.Time
}

// MaxScheduleAhead bounds how far in the future a Pix can be scheduled.
const MaxScheduleAhead = 365 * 24 * time.Hour

// NewSchedule validates a scheduling request made on today.
func NewSchedule(key string, account uuid.UUID, pixKey string, amount money.Money, description string,
	start time.Time, freq Frequency, occurrences int, today time.Time) (Schedule, error) {
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	switch {
	case key == "":
		return Schedule{}, fmt.Errorf("%w: idempotency key is required", ErrInvalidArgument)
	case !amount.IsPositive() || amount.Currency() != money.BRL:
		return Schedule{}, fmt.Errorf("%w: amount must be positive BRL", ErrInvalidArgument)
	case freq != Once && freq != Weekly && freq != Monthly:
		return Schedule{}, fmt.Errorf("%w: frequency must be ONCE, WEEKLY or MONTHLY", ErrInvalidArgument)
	case start.Before(today):
		return Schedule{}, fmt.Errorf("%w: the first date cannot be in the past", ErrInvalidArgument)
	case start.Sub(today) > MaxScheduleAhead:
		return Schedule{}, fmt.Errorf("%w: at most one year ahead", ErrInvalidArgument)
	case occurrences < 0 || occurrences > 120 || (freq == Once && occurrences > 1):
		return Schedule{}, fmt.Errorf("%w: invalid number of occurrences", ErrInvalidArgument)
	case len(description) > 140:
		return Schedule{}, fmt.Errorf("%w: description longer than 140 characters", ErrInvalidArgument)
	}
	if freq == Once {
		occurrences = 1
	}
	return Schedule{
		ID: uuid.Must(uuid.NewV7()), IdempotencyKey: key, AccountID: account, Key: pixKey, Amount: amount,
		Description: description, StartDate: start, Frequency: freq, Occurrences: occurrences,
		NextDate: start, Status: ScheduleActive,
	}, nil
}

// OccurrenceDate is the date of the n-th execution (n from 0). Monthly dates
// are computed from the start date, clamped to the month's last day: the
// 31st runs on Feb 28 and on Mar 31 again, never drifting.
func (s Schedule) OccurrenceDate(n int) time.Time {
	switch s.Frequency {
	case Weekly:
		return s.StartDate.AddDate(0, 0, 7*n)
	case Monthly:
		first := time.Date(s.StartDate.Year(), s.StartDate.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
		last := first.AddDate(0, 1, -1).Day()
		return first.AddDate(0, 0, min(s.StartDate.Day(), last)-1)
	}
	return s.StartDate
}

// Due reports whether an execution should run on today.
func (s Schedule) Due(today time.Time) bool {
	return s.Status == ScheduleActive && !s.NextDate.After(today)
}

// TransferKey is the idempotency key of the n-th execution (n from 1).
func (s Schedule) TransferKey(n int) string { return fmt.Sprintf("sched:%s:%d", s.ID, n) }

// Advance records an execution and moves to the next date.
func (s *Schedule) Advance() {
	s.Executed++
	if s.Occurrences > 0 && s.Executed >= s.Occurrences {
		s.Status = ScheduleCompleted
		return
	}
	s.NextDate = s.OccurrenceDate(s.Executed)
}

func (s *Schedule) Cancel() error {
	if s.Status != ScheduleActive {
		return fmt.Errorf("%w: schedule is %s", ErrInvalidArgument, s.Status)
	}
	s.Status = ScheduleCancelled
	return nil
}
