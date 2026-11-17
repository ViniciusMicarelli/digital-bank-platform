// Package domain models a money movement and its saga state machine.
package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

var (
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrNotFound            = errors.New("not found")
	ErrInvalidState        = errors.New("invalid transaction state")
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	// ErrBusy means another worker currently owns the saga; retry shortly.
	ErrBusy = errors.New("transaction is being processed")
)

// RejectedError is a business refusal by a downstream service (insufficient
// funds, limit exceeded, inactive account...). It triggers compensation,
// unlike technical errors, which leave the saga to be retried.
type RejectedError struct {
	Reason string
	Msg    string
}

func (e *RejectedError) Error() string { return e.Reason + ": " + e.Msg }

// Reject builds a RejectedError.
func Reject(reason, msg string) error { return &RejectedError{Reason: reason, Msg: msg} }

// AsRejected returns the RejectedError in err's chain, if any.
func AsRejected(err error) (*RejectedError, bool) {
	var r *RejectedError
	ok := errors.As(err, &r)
	return r, ok
}

type Type string

const (
	TypeInternalTransfer Type = "INTERNAL_TRANSFER"
	TypePixOut           Type = "PIX_OUT"
	TypePixIn            Type = "PIX_IN"
)

type Status string

const (
	StatusPending      Status = "PENDING"
	StatusReserved     Status = "RESERVED"
	StatusCompleted    Status = "COMPLETED"
	StatusCompensating Status = "COMPENSATING"
	StatusFailed       Status = "FAILED"
)

// Terminal reports whether no more steps will run.
func (s Status) Terminal() bool { return s == StatusCompleted || s == StatusFailed }

// Transaction is a saga instance.
type Transaction struct {
	ID             uuid.UUID
	IdempotencyKey string
	Type           Type
	Status         Status
	From, To       uuid.UUID
	Amount         money.Money
	Description    string
	Metadata       map[string]string
	LimitConsumed  bool
	HoldID         uuid.UUID
	EntryID        uuid.UUID
	FailureReason  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// New validates a request and returns a PENDING transaction.
func New(key string, typ Type, from, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (Transaction, error) {
	switch {
	case key == "":
		return Transaction{}, fmt.Errorf("%w: idempotency key is required", ErrInvalidArgument)
	case typ != TypeInternalTransfer && typ != TypePixOut && typ != TypePixIn:
		return Transaction{}, fmt.Errorf("%w: unknown type %q", ErrInvalidArgument, typ)
	case from == uuid.Nil || to == uuid.Nil:
		return Transaction{}, fmt.Errorf("%w: from and to accounts are required", ErrInvalidArgument)
	case from == to:
		return Transaction{}, fmt.Errorf("%w: from and to must differ", ErrInvalidArgument)
	case !amount.IsPositive():
		return Transaction{}, fmt.Errorf("%w: amount must be positive", ErrInvalidArgument)
	case len(description) > 140:
		return Transaction{}, fmt.Errorf("%w: description longer than 140 characters", ErrInvalidArgument)
	}
	if metadata == nil {
		metadata = map[string]string{}
	}
	return Transaction{
		ID:             uuid.Must(uuid.NewV7()),
		IdempotencyKey: key,
		Type:           typ,
		Status:         StatusPending,
		From:           from,
		To:             to,
		Amount:         amount,
		Description:    description,
		Metadata:       maps.Clone(metadata),
	}, nil
}

// ConsumesLimit reports whether the debited account's limits apply.
func (t Transaction) ConsumesLimit() bool { return t.Type != TypePixIn }

// TwoPhase reports whether the money is held before an external confirmation.
func (t Transaction) TwoPhase() bool { return t.Type == TypePixOut }

// Idempotency keys for downstream calls, derived from the transaction id so
// that replaying a step never moves money twice.
func (t Transaction) EntryKey() string { return "txn:" + t.ID.String() + ":entry" }
func (t Transaction) HoldKey() string  { return "txn:" + t.ID.String() + ":hold" }

// Fingerprint identifies the request for idempotency checks.
func (t Transaction) Fingerprint() []byte {
	h := sha256.New()
	h.Write([]byte(t.Type))
	h.Write(t.From[:])
	h.Write(t.To[:])
	_ = binary.Write(h, binary.BigEndian, t.Amount.Minor())
	h.Write([]byte(t.Amount.Currency()))
	h.Write([]byte{0})
	h.Write([]byte(t.Description))
	for _, k := range slices.Sorted(maps.Keys(t.Metadata)) {
		h.Write([]byte{0})
		h.Write([]byte(k))
		h.Write([]byte{1})
		h.Write([]byte(t.Metadata[k]))
	}
	return h.Sum(nil)
}

// StartCompensation moves a non-terminal transaction to COMPENSATING.
func (t *Transaction) StartCompensation(reason string) error {
	if t.Status.Terminal() || t.Status == StatusCompensating {
		return fmt.Errorf("%w: cannot compensate a %s transaction", ErrInvalidState, t.Status)
	}
	t.Status, t.FailureReason = StatusCompensating, reason
	return nil
}
