package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	// StatusScheduled waits for its date; the amount is quoted then.
	StatusScheduled Status = "SCHEDULED"
	StatusCreated   Status = "CREATED"
	// StatusFundsReserved: limit consumed and money held in the ledger.
	StatusFundsReserved Status = "FUNDS_RESERVED"
	// StatusSentToClearing is recorded before calling the clearing network,
	// so a crash in between is retried with the same id (idempotent there).
	StatusSentToClearing Status = "SENT_TO_CLEARING"
	StatusCompleted      Status = "COMPLETED"
	// StatusRejected: refused before money moved (funds, limits, registry).
	StatusRejected Status = "REJECTED"
	// StatusFailed: the clearing network refused it; the hold was released.
	StatusFailed    Status = "FAILED"
	StatusCancelled Status = "CANCELLED"
)

func (s Status) Terminal() bool {
	return s == StatusCompleted || s == StatusRejected || s == StatusFailed || s == StatusCancelled
}

// Payee is who issued the boleto, as the registry (CIP) knows it.
type Payee struct {
	Name           string
	DocumentMasked string
	BankName       string
}

// Quote is what paying a boleto costs on a date.
type Quote struct {
	Boleto   Boleto
	Payee    Payee
	Nominal  int64
	Fine     int64
	Interest int64
}

func (q Quote) Total() int64 { return q.Nominal + q.Fine + q.Interest }

type Payment struct {
	ID             uuid.UUID
	IdempotencyKey string
	AccountID      uuid.UUID
	Status         Status
	FailureReason  string
	Barcode        string
	Line           string
	Payee          Payee
	DueDate        time.Time
	Nominal        int64
	Fine           int64
	Interest       int64
	// ScheduledFor is the payment date (São Paulo); today for immediate ones.
	ScheduledFor  time.Time
	TransactionID uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (p Payment) Amount() int64 { return p.Nominal + p.Fine + p.Interest }

// SagaKey is the idempotency key of the money movement.
func (p Payment) SagaKey() string { return "boleto:" + p.ID.String() }

// Apply sets the amounts of a quote (when a scheduled payment runs).
func (p *Payment) Apply(q Quote) {
	p.Payee, p.DueDate, p.Nominal, p.Fine, p.Interest = q.Payee, q.Boleto.DueDate, q.Nominal, q.Fine, q.Interest
}

func (p *Payment) Cancel() error {
	if p.Status != StatusScheduled {
		return fmt.Errorf("%w: only scheduled payments can be cancelled (this one is %s)", ErrInvalidState, p.Status)
	}
	p.Status = StatusCancelled
	return nil
}
