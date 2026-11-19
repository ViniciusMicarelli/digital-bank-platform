package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RefusedError is a business refusal (FAILED_PRECONDITION with a reason).
type RefusedError struct{ Reason, Msg string }

func (e *RefusedError) Error() string { return e.Reason + ": " + e.Msg }

func Refuse(reason, msg string) error { return &RefusedError{Reason: reason, Msg: msg} }

// AsRefused unwraps a refusal.
func AsRefused(err error) (*RefusedError, bool) {
	var r *RefusedError
	ok := errors.As(err, &r)
	return r, ok
}

// ReasonFraudSuspected blocks a card whose holder disputed a purchase they
// do not recognize.
const ReasonFraudSuspected = "FRAUD_SUSPECTED"

// DisputeWindow is how long after a purchase it can be disputed.
const DisputeWindow = 90 * 24 * time.Hour

// RefundSource says why money goes back to the customer.
type RefundSource string

const (
	RefundMerchant RefundSource = "MERCHANT"
	RefundDispute  RefundSource = "DISPUTE"
)

type RefundStatus string

const (
	RefundPending   RefundStatus = "PENDING" // the money is on its way (retried)
	RefundCompleted RefundStatus = "COMPLETED"
	RefundFailed    RefundStatus = "FAILED"
)

// Refund gives (part of) a cleared purchase back: to the checking account
// (debit) or as a credit on the invoice (credit).
type Refund struct {
	ID              uuid.UUID
	AuthorizationID uuid.UUID
	AccountID       uuid.UUID
	Function        Function
	MerchantName    string
	Amount          int64
	Source          RefundSource
	Reference       string
	Status          RefundStatus
	FailureReason   string
	CreatedAt       time.Time
}

// Key is the idempotency key of the money movement.
func (r Refund) Key() string { return "card-refund:" + r.ID.String() }

// Refundable is what is left to give back on a purchase, given the amount
// under an open dispute.
func (a Authorization) Refundable(disputed int64) int64 { return max(a.Cleared-a.Refunded-disputed, 0) }

// NewRefund checks a refund against what is left of the purchase.
func NewRefund(a Authorization, amount int64, source RefundSource, reference string, disputed int64, now time.Time) (Refund, error) {
	switch {
	case reference == "":
		return Refund{}, fmt.Errorf("%w: a refund reference is required", ErrInvalidArgument)
	case a.Status != Cleared:
		return Refund{}, Refuse("NOT_CLEARED", "only a cleared purchase can be refunded")
	case amount <= 0:
		return Refund{}, fmt.Errorf("%w: the amount must be positive", ErrInvalidArgument)
	case amount > a.Refundable(disputed):
		return Refund{}, Refuse("REFUND_TOO_LARGE", fmt.Sprintf("at most %d centavos can be refunded", a.Refundable(disputed)))
	}
	return Refund{ID: uuid.Must(uuid.NewV7()), AuthorizationID: a.ID, AccountID: a.AccountID, Function: a.Purchase.Function,
		MerchantName: a.Purchase.MerchantName, Amount: amount, Source: source, Reference: reference, Status: RefundPending, CreatedAt: now}, nil
}

// DisputeReason is why the customer disputes a purchase.
type DisputeReason string

const (
	// DisputeNotRecognized is fraud: the customer did not make the purchase.
	DisputeNotRecognized DisputeReason = "NOT_RECOGNIZED"
	DisputeNotReceived   DisputeReason = "NOT_RECEIVED"
	DisputeDuplicate     DisputeReason = "DUPLICATE"
	DisputeWrongAmount   DisputeReason = "WRONG_AMOUNT"
	DisputeCancelled     DisputeReason = "CANCELLED"
)

var disputeReasons = []DisputeReason{DisputeNotRecognized, DisputeNotReceived, DisputeDuplicate, DisputeWrongAmount, DisputeCancelled}

type DisputeStatus string

const (
	DisputeOpen DisputeStatus = "OPEN"
	DisputeWon  DisputeStatus = "WON"
	DisputeLost DisputeStatus = "LOST"
)

// Dispute is a customer's chargeback request, decided by an analyst.
type Dispute struct {
	ID              uuid.UUID
	AuthorizationID uuid.UUID
	CardID          uuid.UUID
	AccountID       uuid.UUID
	Reason          DisputeReason
	Amount          int64
	Description     string
	Status          DisputeStatus
	DecidedBy       string
	DecisionReason  string
	CreatedAt       time.Time
	DecidedAt       *time.Time
	// From the purchase, for display.
	MerchantName string
	PurchasedAt  time.Time
}

// NewDispute opens a dispute on a cleared purchase, within the window, for
// at most what was not refunded (all of it by default). open says whether
// the purchase already has an open dispute.
func NewDispute(a Authorization, reason DisputeReason, amount int64, description string, open bool, now time.Time) (Dispute, error) {
	switch {
	case !slices.Contains(disputeReasons, reason):
		return Dispute{}, fmt.Errorf("%w: unknown dispute reason", ErrInvalidArgument)
	case len(description) > 500:
		return Dispute{}, fmt.Errorf("%w: description longer than 500 characters", ErrInvalidArgument)
	case a.Status != Cleared:
		return Dispute{}, Refuse("NOT_CLEARED", "only a cleared purchase can be disputed")
	case now.Sub(a.CreatedAt) > DisputeWindow:
		return Dispute{}, Refuse("DISPUTE_WINDOW_CLOSED", "purchases can be disputed for 90 days")
	case open:
		return Dispute{}, Refuse("DISPUTE_ALREADY_OPEN", "this purchase is already being disputed")
	case a.Refundable(0) == 0:
		return Dispute{}, Refuse("NOTHING_TO_DISPUTE", "this purchase was fully refunded")
	}
	if amount == 0 {
		amount = a.Refundable(0)
	}
	if amount < 0 || amount > a.Refundable(0) {
		return Dispute{}, fmt.Errorf("%w: at most %d centavos can be disputed", ErrInvalidArgument, a.Refundable(0))
	}
	return Dispute{ID: uuid.Must(uuid.NewV7()), AuthorizationID: a.ID, CardID: a.CardID, AccountID: a.AccountID, Reason: reason,
		Amount: amount, Description: strings.TrimSpace(description), Status: DisputeOpen, CreatedAt: now,
		MerchantName: a.Purchase.MerchantName, PurchasedAt: a.CreatedAt}, nil
}

// Decide records the analyst's decision: won refunds the amount, lost needs a reason.
func (d *Dispute) Decide(analyst string, won bool, reason string, now time.Time) error {
	switch {
	case strings.TrimSpace(analyst) == "":
		return fmt.Errorf("%w: analyst_id is required", ErrInvalidArgument)
	case d.Status != DisputeOpen:
		return Refuse("DISPUTE_NOT_OPEN", "this dispute was already decided")
	case !won && strings.TrimSpace(reason) == "":
		return fmt.Errorf("%w: a lost dispute needs a reason", ErrInvalidArgument)
	}
	d.Status, d.DecidedBy, d.DecisionReason, d.DecidedAt = DisputeLost, analyst, strings.TrimSpace(reason), &now
	if won {
		d.Status = DisputeWon
	}
	return nil
}
