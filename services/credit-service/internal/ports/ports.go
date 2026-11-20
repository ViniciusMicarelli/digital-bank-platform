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
	// UpcomingInstallments is the sum of installments not on an invoice yet.
	UpcomingInstallments(ctx context.Context, line uuid.UUID) (int64, error)
	// InstallmentPlans lists purchases with installments still to come, newest first.
	InstallmentPlans(ctx context.Context, line uuid.UUID) ([]domain.InstallmentPlan, error)
	InvoicePlanByKey(ctx context.Context, key string) (*domain.InvoicePlan, error)
	// UnpostedPlanInterest lists plans whose interest is not in the ledger yet.
	UnpostedPlanInterest(ctx context.Context, limit int) ([]domain.InvoicePlan, error)
	MarkPlanInterestPosted(ctx context.Context, id uuid.UUID) error
	LimitRequestByKey(ctx context.Context, key string) (*domain.LimitRequest, error)
	LimitRequest(ctx context.Context, id uuid.UUID) (domain.LimitRequest, error)
	// LimitRequests by line (zero for all) and/or status, newest first.
	LimitRequests(ctx context.Context, line uuid.UUID, status domain.LimitStatus, limit int) ([]domain.LimitRequest, error)
	// HasOverdue reports whether the line has an invoice that went overdue.
	HasOverdue(ctx context.Context, line uuid.UUID) (bool, error)
	RefundByKey(ctx context.Context, key string) (*domain.Refund, error)

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

	InsertInstallments(list []domain.Installment) error
	// InsertLimitRequest fails with LIMIT_REQUEST_IN_REVIEW when one is open.
	InsertLimitRequest(r domain.LimitRequest) error
	LockLimitRequest(id uuid.UUID) (domain.LimitRequest, error)
	SaveLimitRequest(r domain.LimitRequest) error
	// InsertInvoicePlan fails with a refusal when the invoice has a plan already.
	InsertInvoicePlan(p domain.InvoicePlan) error
	// NextInstallments lists the lowest pending installment of each plan of a
	// line (the caller holds the line lock).
	NextInstallments(line uuid.UUID) ([]domain.Installment, error)
	BillInstallment(id, invoice uuid.UUID) error

	// InsertRefund returns false when the key exists already.
	InsertRefund(r domain.Refund) (bool, error)

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
