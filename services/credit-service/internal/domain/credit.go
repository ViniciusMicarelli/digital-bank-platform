// Package domain models credit lines, purchase holds and monthly invoices.
// Amounts are BRL minor units (centavos).
package domain

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
	ErrInvalidState    = errors.New("invalid state")
	ErrConflict        = errors.New("idempotency key reused with a different request")
	ErrAccountInactive = errors.New("account is not active")
)

// RefusedError is a business refusal returned to the customer (422).
type RefusedError struct{ Reason, Msg string }

func (e *RefusedError) Error() string { return e.Reason + ": " + e.Msg }

func Refuse(reason, msg string) error { return &RefusedError{Reason: reason, Msg: msg} }

// Refusal reasons of a hold.
const (
	ReasonInsufficientLimit = "INSUFFICIENT_LIMIT"
	ReasonNoCreditLine      = "NO_CREDIT_LINE"
	ReasonLineBlocked       = "CREDIT_LINE_BLOCKED"
)

const (
	DefaultLimit          int64 = 1_000_00
	DefaultInterestRateBP       = 800 // 8% a month
	FineBP                      = 200 // 2%, the maximum fine in Brazil
	DueAfterClosingDays         = 10
	HoldLifetime                = 7 * 24 * time.Hour
)

var saoPaulo = func() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*3600)
	}
	return loc
}()

// Today is the civil date in São Paulo, as midnight UTC.
func Today(now time.Time) time.Time {
	y, m, d := now.In(saoPaulo).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ---- line ----

type LineStatus string

const (
	LineActive  LineStatus = "ACTIVE"
	LineBlocked LineStatus = "BLOCKED"
)

type Line struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	IdempotencyKey string
	Status         LineStatus
	Limit          int64
	// Used is held purchases plus everything owed on unpaid invoices.
	Used           int64
	ClosingDay     int
	InterestRateBP int
	CreatedAt      time.Time
}

func NewLine(key string, account uuid.UUID, closingDay int, limit int64) (Line, error) {
	switch {
	case key == "":
		return Line{}, fmt.Errorf("%w: idempotency key is required", ErrInvalidArgument)
	case closingDay < 1 || closingDay > 28:
		return Line{}, fmt.Errorf("%w: closing day must be between 1 and 28", ErrInvalidArgument)
	case limit <= 0:
		return Line{}, fmt.Errorf("%w: limit must be positive", ErrInvalidArgument)
	}
	return Line{ID: uuid.Must(uuid.NewV7()), AccountID: account, IdempotencyKey: key, Status: LineActive,
		Limit: limit, ClosingDay: closingDay, InterestRateBP: DefaultInterestRateBP}, nil
}

func (l Line) Available() int64 { return max(l.Limit-l.Used, 0) }

// Reserve takes amount from the available limit, or returns a refusal reason.
func (l *Line) Reserve(amount int64) string {
	switch {
	case l.Status != LineActive:
		return ReasonLineBlocked
	case amount > l.Available():
		return ReasonInsufficientLimit
	}
	l.Used += amount
	return ""
}

// Free gives amount back to the limit (a release, a payment, a smaller capture).
func (l *Line) Free(amount int64) { l.Used = max(l.Used-amount, 0) }

// ---- holds ----

type HoldStatus string

const (
	HoldReserved HoldStatus = "RESERVED"
	HoldCaptured HoldStatus = "CAPTURED"
	HoldReleased HoldStatus = "RELEASED"
	HoldFailed   HoldStatus = "FAILED"
)

// Hold is the limit set aside for one card authorization.
type Hold struct {
	ID             uuid.UUID
	IdempotencyKey string
	LineID         uuid.UUID // Nil when the account has no line
	AccountID      uuid.UUID
	Status         HoldStatus
	FailureReason  string
	Amount         int64
	Captured       int64
	Description    string
	Metadata       map[string]string
	CreatedAt      time.Time
	// Installments is 1, or 2 to 12 for an interest-free installment purchase.
	Installments int
}

// Installment purchases.
const (
	MaxInstallments = 12
	// MinInstallment is the smallest installment a purchase can be split into.
	MinInstallment = 5_00
)

// ValidInstallments checks how a purchase of amount may be split.
func ValidInstallments(amount int64, n int) error {
	switch {
	case n < 1 || n > MaxInstallments:
		return fmt.Errorf("%w: installments must be between 1 and %d", ErrInvalidArgument, MaxInstallments)
	case n > 1 && amount/int64(n) < MinInstallment:
		return fmt.Errorf("%w: each installment must be at least R$ 5,00", ErrInvalidArgument)
	}
	return nil
}

// Split divides amount into n installments; the first takes the remainder
// so they add up exactly.
func Split(amount int64, n int) []int64 {
	out := make([]int64, n)
	each := amount / int64(n)
	for i := range out {
		out[i] = each
	}
	out[0] += amount - each*int64(n)
	return out
}

// Installment is one part of an installment purchase (HoldID) or of an
// invoice plan (PlanID), billed on one invoice.
type Installment struct {
	ID          uuid.UUID
	LineID      uuid.UUID
	HoldID      uuid.UUID
	PlanID      uuid.UUID
	Number      int
	Total       int
	Amount      int64
	Description string
	InvoiceID   *uuid.UUID
	CreatedAt   time.Time
}

// Label is how the installment shows on the invoice, e.g. "LOJA DE TV (2/10)".
func (i Installment) Label() string {
	return fmt.Sprintf("%s (%d/%d)", i.Description, i.Number, i.Total)
}

// InstallmentPlan summarizes an installment purchase.
type InstallmentPlan struct {
	HoldID      uuid.UUID
	Description string
	Total       int64
	Count       int
	Billed      int
	Installment int64
	Remaining   int64
	PurchasedAt time.Time
}

// CaptureKey is the idempotency key of the ledger posting of a capture.
func (h Hold) CaptureKey() string { return "credit-capture:" + h.ID.String() }

// ---- invoices ----

type InvoiceStatus string

const (
	InvoiceOpen    InvoiceStatus = "OPEN"
	InvoiceClosed  InvoiceStatus = "CLOSED"
	InvoicePaid    InvoiceStatus = "PAID"
	InvoiceOverdue InvoiceStatus = "OVERDUE"
)

type Invoice struct {
	ID          uuid.UUID
	LineID      uuid.UUID
	Status      InvoiceStatus
	ClosingDate time.Time
	DueDate     time.Time
	Total       int64
	Paid        int64
	// Pending is the sum of payments sent to the ledger and not yet settled.
	Pending int64
	// Charges are the fine and interest of an invoice that went overdue.
	Charges int64
}

func (i Invoice) Remaining() int64 { return i.Total - i.Paid }

// FirstClosing is the first closingDay strictly after today.
func FirstClosing(closingDay int, today time.Time) time.Time {
	c := time.Date(today.Year(), today.Month(), closingDay, 0, 0, 0, 0, time.UTC)
	if !c.After(today) {
		c = c.AddDate(0, 1, 0)
	}
	return c
}

func NewInvoice(line uuid.UUID, closing time.Time) Invoice {
	return Invoice{ID: uuid.Must(uuid.NewV7()), LineID: line, Status: InvoiceOpen,
		ClosingDate: closing, DueDate: closing.AddDate(0, 0, DueAfterClosingDays)}
}

// Next is the invoice that opens when i closes (closing days are <= 28, so
// adding a month never skips one).
func (i Invoice) Next() Invoice { return NewInvoice(i.LineID, i.ClosingDate.AddDate(0, 1, 0)) }

// Close ends the period: nothing to pay means paid already.
func (i *Invoice) Close() error {
	if i.Status != InvoiceOpen {
		return fmt.Errorf("%w: cannot close a %s invoice", ErrInvalidState, i.Status)
	}
	i.Status = InvoiceClosed
	if i.Remaining() <= 0 && i.Pending == 0 {
		i.Status = InvoicePaid
	}
	return nil
}

// StartPayment reserves amount of the remaining balance for a payment in
// flight, so concurrent payments can never pay more than is owed.
func (i *Invoice) StartPayment(amount int64) error {
	switch {
	case amount <= 0:
		return fmt.Errorf("%w: amount must be positive", ErrInvalidArgument)
	case i.Status != InvoiceOpen && i.Status != InvoiceClosed:
		return Refuse("INVOICE_NOT_PAYABLE", fmt.Sprintf("the invoice is %s", i.Status))
	case amount > i.Remaining()-i.Pending:
		return Refuse("PAYMENT_TOO_LARGE", fmt.Sprintf("at most %d centavos are left to pay", i.Remaining()-i.Pending))
	}
	i.Pending += amount
	return nil
}

// SettlePayment applies a payment that went through (ok) or not.
func (i *Invoice) SettlePayment(amount int64, ok bool) {
	i.Pending -= amount
	if !ok {
		return
	}
	i.Paid += amount
	if i.Status == InvoiceClosed && i.Remaining() <= 0 && i.Pending == 0 {
		i.Status = InvoicePaid
	}
}

// Overdue reports whether a closed invoice passed its due date unpaid.
func (i Invoice) Overdue(today time.Time) bool {
	return i.Status == InvoiceClosed && today.After(i.DueDate) && i.Pending == 0 && i.Remaining() > 0
}

// Charges on an unpaid balance: the 2% fine and a month of interest.
func Charges(remaining int64, interestBP int) (fine, interest int64) {
	return percent(remaining, FineBP), percent(remaining, interestBP)
}

// percent returns amount * bp / 10000, rounded half up.
func percent(amount int64, bp int) int64 { return (amount*int64(bp) + 5000) / 10000 }

type ItemKind string

const (
	ItemPurchase       ItemKind = "PURCHASE"
	ItemPayment        ItemKind = "PAYMENT"
	ItemBalanceCarried ItemKind = "BALANCE_CARRIED"
	ItemFine           ItemKind = "FINE"
	ItemInterest       ItemKind = "INTEREST"
	// Credits: a card refund, and a credit carried from the previous invoice.
	ItemRefund          ItemKind = "REFUND"
	ItemCreditCarriedIn ItemKind = "CREDIT_CARRIED_IN"
	// A credit larger than the invoice, moved to the next one.
	ItemCreditCarriedOut ItemKind = "CREDIT_CARRIED_OUT"
	// The balance of a closed invoice settled by an installment plan.
	ItemInvoicePlan ItemKind = "INVOICE_PLAN"
)

// Refund is a card refund credited to an invoice.
type Refund struct {
	ID             uuid.UUID
	IdempotencyKey string
	LineID         uuid.UUID
	AccountID      uuid.UUID
	InvoiceID      uuid.UUID
	Amount         int64
	Description    string
	CreatedAt      time.Time
}

type Item struct {
	ID          uuid.UUID
	InvoiceID   uuid.UUID
	Kind        ItemKind
	Amount      int64
	Description string
	// Reference makes an item unique (hold, payment or invoice id).
	Reference  string
	OccurredAt time.Time
}

func NewItem(invoice uuid.UUID, kind ItemKind, amount int64, description, reference string, at time.Time) Item {
	return Item{ID: uuid.Must(uuid.NewV7()), InvoiceID: invoice, Kind: kind, Amount: amount,
		Description: description, Reference: reference, OccurredAt: at}
}

// ---- payments ----

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "PENDING"
	PaymentCompleted PaymentStatus = "COMPLETED"
	PaymentFailed    PaymentStatus = "FAILED"
)

type Payment struct {
	ID             uuid.UUID
	IdempotencyKey string
	InvoiceID      uuid.UUID
	AccountID      uuid.UUID
	Status         PaymentStatus
	FailureReason  string
	Amount         int64
	CreatedAt      time.Time
}

// TransferKey is the idempotency key of the money movement of a payment.
func (p Payment) TransferKey() string { return "credit-payment:" + p.ID.String() }

// ---- minimum payment and invoice plans ----

// Brazilian practice: paying less than this lets the rest roll over with
// charges; the minimum is 15% of the balance, at least R$ 50 (or all of it).
const (
	MinimumPaymentBP  = 1500
	MinimumPaymentMin = 50_00
)

// MinimumPayment of a balance.
func MinimumPayment(remaining int64) int64 {
	if remaining <= 0 {
		return 0
	}
	return min(remaining, max(percent(remaining, MinimumPaymentBP), MinimumPaymentMin))
}

// Invoice plans ("parcelamento da fatura").
const (
	// InvoicePlanRateBP is the monthly interest of a plan (4.99%), below the
	// revolving rate, as banks offer it.
	InvoicePlanRateBP   = 499
	MinInvoicePlan      = 100_00
	MinPlanInstallments = 2
	MaxPlanInstallments = 12
)

// PlanQuote is one way to split a balance.
type PlanQuote struct {
	Installments int
	Installment  int64
	Total        int64
	Interest     int64
	MonthlyBP    int
	// AnnualCostBP is the CET (custo efetivo total) a year, in basis points.
	AnnualCostBP int
}

// QuotePlan splits principal into n equal installments (Price table),
// rounded up to the centavo.
func QuotePlan(principal int64, n, monthlyBP int) PlanQuote {
	i := float64(monthlyBP) / 10_000
	pmt := float64(principal) / float64(n)
	if i > 0 {
		pmt = float64(principal) * i / (1 - math.Pow(1+i, -float64(n)))
	}
	installment := int64(math.Ceil(pmt))
	total := installment * int64(n)
	return PlanQuote{Installments: n, Installment: installment, Total: total, Interest: total - principal, MonthlyBP: monthlyBP,
		AnnualCostBP: int(math.Round((math.Pow(1+i, 12) - 1) * 10_000))}
}

// Plannable checks that a closed invoice can be split.
func (i Invoice) Plannable() error {
	switch {
	case i.Status != InvoiceClosed:
		return Refuse("INVOICE_NOT_PLANNABLE", "only a closed invoice not yet due can be split")
	case i.Pending > 0:
		return Refuse("PAYMENT_IN_PROGRESS", "a payment of this invoice is in progress")
	case i.Remaining() < MinInvoicePlan:
		return Refuse("INVOICE_TOO_SMALL", "invoices of at least R$ 100,00 can be split")
	}
	return nil
}

// InvoicePlan is a closed invoice split into installments.
type InvoicePlan struct {
	ID             uuid.UUID
	IdempotencyKey string
	InvoiceID      uuid.UUID
	LineID         uuid.UUID
	Principal      int64
	Installments   int
	Installment    int64
	MonthlyBP      int
	CreatedAt      time.Time
}

// Interest is what the plan adds to the debt.
func (p InvoicePlan) Interest() int64 { return p.Installment*int64(p.Installments) - p.Principal }

// InterestKey is the idempotency key of the ledger posting of the interest.
func (p InvoicePlan) InterestKey() string { return "invoice-plan-interest:" + p.ID.String() }
