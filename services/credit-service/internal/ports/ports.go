// Package ports declares the credit service's dependencies.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/domain"
)

// Store reads outside transactions and runs units of work in one.
type Store interface {
	InTx(ctx context.Context, fn func(Tx) error) error

	Line(ctx context.Context, id uuid.UUID) (domain.Line, error)
	LineByAccount(ctx context.Context, account uuid.UUID) (domain.Line, error)
	Invoices(ctx context.Context, line uuid.UUID) ([]domain.Invoice, error)
	Invoice(ctx context.Context, id uuid.UUID) (domain.Invoice, error)
	Items(ctx context.Context, invoice uuid.UUID) ([]domain.Item, error)
	Hold(ctx context.Context, id uuid.UUID) (domain.Hold, error)

	// LinesDue lists lines with an invoice to close or to make overdue.
	LinesDue(ctx context.Context, today time.Time, limit int) ([]uuid.UUID, error)
	// UnpostedCharges lists overdue invoices whose charges are not in the ledger yet.
	UnpostedCharges(ctx context.Context, limit int) ([]domain.Invoice, error)
	MarkChargesPosted(ctx context.Context, invoice uuid.UUID) error
	ExpiredHolds(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error)
	PendingPayments(ctx context.Context, before time.Time, limit int) ([]domain.Payment, error)
}

// Tx is a unit of work. Lock* methods take row locks (always line before
// invoice, to avoid deadlocks).
type Tx interface {
	// InsertLine returns false if the account already has a line.
	InsertLine(l domain.Line) (bool, error)
	LockLine(id uuid.UUID) (domain.Line, error)
	LockLineByAccount(account uuid.UUID) (domain.Line, error)
	SaveLine(l domain.Line) error

	InsertInvoice(i domain.Invoice) error
	LockOpenInvoice(line uuid.UUID) (domain.Invoice, error)
	LockInvoice(id uuid.UUID) (domain.Invoice, error)
	LockClosedInvoices(line uuid.UUID) ([]domain.Invoice, error)
	SaveInvoice(i domain.Invoice) error
	// SetCharges records the charges of an invoice that went overdue.
	SetCharges(invoice uuid.UUID, charges int64) error
	AddItem(it domain.Item) error

	HoldByKey(key string) (*domain.Hold, error)
	LockHold(id uuid.UUID) (domain.Hold, error)
	InsertHold(h domain.Hold) error
	SaveHold(h domain.Hold) error

	PaymentByKey(key string) (*domain.Payment, error)
	LockPayment(id uuid.UUID) (domain.Payment, error)
	InsertPayment(p domain.Payment) error
	SavePayment(p domain.Payment) error

	Publish(m outbox.Message) error
}

type Accounts interface {
	// Active returns domain.ErrNotFound for an unknown account.
	Active(ctx context.Context, account uuid.UUID) (bool, error)
}

// TxResult is the outcome of a transaction-service call.
type TxResult struct {
	ID            uuid.UUID
	Status        string // COMPLETED or FAILED (one-step types)
	FailureReason string
}

// Transactions posts one-step movements through transaction-service.
type Transactions interface {
	Post(ctx context.Context, key, typ string, from, to uuid.UUID, amount int64, description string, metadata map[string]string) (TxResult, error)
}
