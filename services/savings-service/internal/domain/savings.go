// Package domain holds the savings box rules: limits, movements and the
// daily yield.
package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("idempotency key reused with other values")
)

// RefusedError is a business refusal (FAILED_PRECONDITION).
type RefusedError struct{ Reason, Msg string }

func (e *RefusedError) Error() string { return e.Reason + ": " + e.Msg }

func Refuse(reason, msg string) error { return &RefusedError{Reason: reason, Msg: msg} }

func AsRefused(err error) (*RefusedError, bool) {
	var r *RefusedError
	ok := errors.As(err, &r)
	return r, ok
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidArgument, fmt.Sprintf(format, a...))
}

// MaxBoxes per checking account.
const MaxBoxes = 20

// Brasilia is the bank's day (Brazil has had no daylight saving since 2019).
var Brasilia = time.FixedZone("BRT", -3*60*60)

// Day is the calendar day of t in Brasília, at midnight UTC (a DATE).
func Day(t time.Time) time.Time {
	y, m, d := t.In(Brasilia).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// EndOfDay is the instant a Brasília day ends.
func EndOfDay(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, Brasilia)
}

type Status string

const (
	Active Status = "ACTIVE"
	Closed Status = "CLOSED"
)

// Box is a savings box ("caixinha").
type Box struct {
	ID              uuid.UUID
	IdempotencyKey  string
	AccountID       uuid.UUID
	Name            string
	Goal            int64
	Status          Status
	Yielded         int64
	YieldCarryMicro int64
	LastAccrued     time.Time
	CreatedAt       time.Time
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 40 {
		return "", invalid("the name has 1 to 40 characters")
	}
	return name, nil
}

// NewBox opens a box; it yields from the day after it is created.
func NewBox(key string, account uuid.UUID, name string, goal int64, now time.Time) (Box, error) {
	if key == "" || account == uuid.Nil {
		return Box{}, invalid("idempotency key and account are required")
	}
	name, err := cleanName(name)
	if err != nil {
		return Box{}, err
	}
	if goal < 0 {
		return Box{}, invalid("the goal cannot be negative")
	}
	return Box{ID: uuid.Must(uuid.NewV7()), IdempotencyKey: key, AccountID: account, Name: name, Goal: goal,
		Status: Active, LastAccrued: Day(now), CreatedAt: now}, nil
}

// Update renames the box and changes (or removes, with 0) its goal.
func (b *Box) Update(name string, goal int64) error {
	if b.Status != Active {
		return Refuse("BOX_CLOSED", "this box is closed")
	}
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	if goal < 0 {
		return invalid("the goal cannot be negative")
	}
	b.Name, b.Goal = name, goal
	return nil
}

type Kind string

const (
	Deposit    Kind = "DEPOSIT"
	Withdrawal Kind = "WITHDRAWAL"
	Yield      Kind = "YIELD"
)

type MovementStatus string

const (
	Pending   MovementStatus = "PENDING"
	Completed MovementStatus = "COMPLETED"
	Failed    MovementStatus = "FAILED"
)

// Movement is money going into or out of a box.
type Movement struct {
	ID             uuid.UUID
	BoxID          uuid.UUID
	IdempotencyKey string
	Kind           Kind
	Amount         int64
	Status         MovementStatus
	FailureReason  string
	CreatedAt      time.Time
}

// TransferKey is the idempotency key of the money movement.
func (m Movement) TransferKey() string { return "savings:" + m.ID.String() }

// NewMovement checks a deposit or withdrawal request against the box.
func NewMovement(b Box, key string, kind Kind, amount int64, now time.Time) (Movement, error) {
	switch {
	case key == "":
		return Movement{}, invalid("idempotency key is required")
	case amount <= 0:
		return Movement{}, invalid("the amount must be positive")
	case b.Status != Active && kind != Withdrawal:
		return Movement{}, Refuse("BOX_CLOSED", "this box is closed")
	}
	return Movement{ID: uuid.Must(uuid.NewV7()), BoxID: b.ID, IdempotencyKey: key, Kind: kind, Amount: amount,
		Status: Pending, CreatedAt: now}, nil
}

// DailyRate is the daily rate equivalent to a yearly rate in basis points
// (compounded every calendar day).
func DailyRate(annualBP int) float64 { return math.Pow(1+float64(annualBP)/10_000, 1.0/365) - 1 }

// Accrue is the yield of balance over days, in whole centavos, carrying the
// fraction of a centavo (in millionths) to the next accrual so nothing is
// lost to rounding.
func Accrue(balance int64, annualBP, days int, carryMicro int64) (amount, carry int64) {
	if balance <= 0 || days <= 0 {
		return 0, carryMicro
	}
	factor := math.Pow(1+DailyRate(annualBP), float64(days)) - 1
	micro := int64(math.Floor(float64(balance)*factor*1e6)) + carryMicro
	return micro / 1_000_000, micro % 1_000_000
}
