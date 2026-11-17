package domain

import (
	"fmt"
	"time"
)

// Limits are maximum outgoing amounts in minor units.
type Limits struct {
	PerTransaction int64
	Daily          int64
	Nightly        int64 // total during one night window
}

// DefaultLimits apply to new accounts.
var DefaultLimits = Limits{
	PerTransaction: 500_000,   // R$ 5.000,00
	Daily:          2_000_000, // R$ 20.000,00
	Nightly:        100_000,   // R$ 1.000,00
}

// MaxDailyLimit caps what a customer can configure without a manual review.
const MaxDailyLimit = 10_000_000 // R$ 100.000,00

// Validate checks that the limits are positive and consistent.
func (l Limits) Validate() error {
	switch {
	case l.PerTransaction <= 0 || l.Daily <= 0 || l.Nightly <= 0:
		return fmt.Errorf("%w: limits must be positive", ErrInvalidArgument)
	case l.PerTransaction > l.Daily:
		return fmt.Errorf("%w: per-transaction limit exceeds daily limit", ErrInvalidArgument)
	case l.Nightly > l.Daily:
		return fmt.Errorf("%w: nightly limit exceeds daily limit", ErrInvalidArgument)
	case l.Daily > MaxDailyLimit:
		return fmt.Errorf("%w: daily limit above %d requires review", ErrInvalidArgument, MaxDailyLimit)
	}
	return nil
}

// Usage is what was already consumed in the current day and night window.
type Usage struct {
	Daily   int64
	Nightly int64
}

// Check returns ErrLimitExceeded when amount does not fit the limits.
// atNight tells whether the transaction happens inside the night window.
func (l Limits) Check(amount int64, used Usage, atNight bool) error {
	if amount <= 0 {
		return fmt.Errorf("%w: amount must be positive", ErrInvalidArgument)
	}
	if amount > l.PerTransaction {
		return fmt.Errorf("%w: per-transaction limit", ErrLimitExceeded)
	}
	if used.Daily+amount > l.Daily {
		return fmt.Errorf("%w: daily limit", ErrLimitExceeded)
	}
	if atNight && used.Nightly+amount > l.Nightly {
		return fmt.Errorf("%w: nightly limit", ErrLimitExceeded)
	}
	return nil
}

// Location is the bank's local time zone; limits reset at local midnight.
var Location = mustLoad("America/Sao_Paulo")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// Night window boundaries, local time (Central Bank rule for Pix).
const (
	nightStartHour = 20
	nightEndHour   = 6
)

// Window locates t in the bank calendar: its local date and, when t is inside
// the night window, the date on which that night started.
func Window(t time.Time) (localDate time.Time, nightOf *time.Time) {
	lt := t.In(Location)
	day := time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, time.UTC)
	switch {
	case lt.Hour() >= nightStartHour:
		return day, &day
	case lt.Hour() < nightEndHour:
		prev := day.AddDate(0, 0, -1)
		return day, &prev
	}
	return day, nil
}
