package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

// HoldStatus is the lifecycle of a two-phase movement.
type HoldStatus string

const (
	HoldActive   HoldStatus = "ACTIVE"
	HoldCaptured HoldStatus = "CAPTURED"
	HoldReleased HoldStatus = "RELEASED"
)

// Hold reserves money on an account (available -> pending) until it is
// captured (posted to the counter account) or released.
type Hold struct {
	ID               uuid.UUID
	IdempotencyKey   string
	AccountID        uuid.UUID
	CounterAccountID uuid.UUID
	Amount           money.Money
	Status           HoldStatus
	CaptureEntryID   uuid.UUID
	Description      string
	CreatedAt        time.Time
}

// NewHold validates a hold request.
func NewHold(key string, account, counter uuid.UUID, amount money.Money, description string) (Hold, error) {
	switch {
	case key == "":
		return Hold{}, fmt.Errorf("%w: idempotency key is required", ErrInvalidArgument)
	case account == uuid.Nil || counter == uuid.Nil:
		return Hold{}, fmt.Errorf("%w: account and counter account are required", ErrInvalidArgument)
	case account == counter:
		return Hold{}, fmt.Errorf("%w: account and counter account must differ", ErrInvalidArgument)
	case !amount.IsPositive():
		return Hold{}, fmt.Errorf("%w: hold amount must be positive", ErrInvalidArgument)
	}
	return Hold{
		ID:               uuid.Must(uuid.NewV7()),
		IdempotencyKey:   key,
		AccountID:        account,
		CounterAccountID: counter,
		Amount:           amount,
		Status:           HoldActive,
		Description:      description,
	}, nil
}

// Fingerprint identifies the request for idempotency checks.
func (h Hold) Fingerprint() []byte {
	s := sha256.New()
	s.Write(h.AccountID[:])
	s.Write(h.CounterAccountID[:])
	_ = binary.Write(s, binary.BigEndian, h.Amount.Minor())
	s.Write([]byte(h.Amount.Currency()))
	s.Write([]byte{0})
	s.Write([]byte(h.Description))
	return s.Sum(nil)
}

// Reserve moves the hold amount from available to pending, enforcing the floor.
func (h Hold) Reserve(acc Account, b *Balance) error {
	if acc.Currency != h.Amount.Currency() {
		return fmt.Errorf("%w: account %s is %s, hold is %s", ErrInvalidArgument, acc.ID, acc.Currency, h.Amount.Currency())
	}
	b.Pending += h.Amount.Minor()
	return checkFloor(acc, *b)
}

// Capture posts amount (at most the held amount) from the account to the
// counter account. Whatever is not captured goes back to available.
func (h *Hold) Capture(amount money.Money, captureKey string, b *Balance) (Entry, error) {
	if h.Status != HoldActive {
		return Entry{}, fmt.Errorf("%w: hold %s is %s", ErrHoldNotActive, h.ID, h.Status)
	}
	if amount.Currency() != h.Amount.Currency() {
		return Entry{}, fmt.Errorf("%w: capture currency differs from hold", ErrInvalidArgument)
	}
	if !amount.IsPositive() || amount.Minor() > h.Amount.Minor() {
		return Entry{}, fmt.Errorf("%w: capture must be > 0 and <= %s", ErrInvalidArgument, h.Amount.Decimal())
	}
	debit, _ := amount.Neg()
	desc := h.Description
	if desc == "" {
		desc = "Capture of hold " + h.ID.String()
	}
	e, err := NewEntry(captureKey, []Posting{
		{AccountID: h.AccountID, Amount: debit},
		{AccountID: h.CounterAccountID, Amount: amount},
	}, desc, map[string]string{"hold_id": h.ID.String()})
	if err != nil {
		return Entry{}, err
	}
	b.Pending -= h.Amount.Minor()
	h.Status = HoldCaptured
	h.CaptureEntryID = e.ID
	return e, nil
}

// Release returns the held amount to available.
func (h *Hold) Release(b *Balance) error {
	if h.Status != HoldActive {
		return fmt.Errorf("%w: hold %s is %s", ErrHoldNotActive, h.ID, h.Status)
	}
	b.Pending -= h.Amount.Minor()
	h.Status = HoldReleased
	return nil
}
