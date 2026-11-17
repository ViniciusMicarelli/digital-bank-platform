package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

// Pix Automático: a payer authorizes a receiver to charge them periodically,
// up to a maximum amount; each charge becomes a scheduled Pix.

var (
	ErrMandateNotActive = errors.New("mandate is not active")
	ErrChargeNotAllowed = errors.New("charge not allowed by the mandate")
)

type MandateStatus string

const (
	MandatePending  MandateStatus = "PENDING"
	MandateActive   MandateStatus = "ACTIVE"
	MandateRejected MandateStatus = "REJECTED"
	MandateRevoked  MandateStatus = "REVOKED"
)

type Mandate struct {
	ID               uuid.UUID
	ReceiverAccount  uuid.UUID
	ReceiverKey      string
	ReceiverName     string
	PayerAccount     uuid.UUID
	MaxAmount        money.Money
	Frequency        Frequency // WEEKLY or MONTHLY
	Description      string
	Status           MandateStatus
	LastChargePeriod string
	CreatedAt        time.Time
}

// NewMandate is the receiver's request, waiting for the payer's approval.
func NewMandate(receiver uuid.UUID, receiverKey, receiverName string, payer uuid.UUID, max money.Money, freq Frequency, description string) (Mandate, error) {
	switch {
	case receiver == payer:
		return Mandate{}, fmt.Errorf("%w: payer and receiver must differ", ErrInvalidArgument)
	case !max.IsPositive() || max.Currency() != money.BRL:
		return Mandate{}, fmt.Errorf("%w: maximum amount must be positive BRL", ErrInvalidArgument)
	case freq != Weekly && freq != Monthly:
		return Mandate{}, fmt.Errorf("%w: frequency must be WEEKLY or MONTHLY", ErrInvalidArgument)
	case description == "" || len(description) > 140:
		return Mandate{}, fmt.Errorf("%w: a description (what is being paid) is required", ErrInvalidArgument)
	}
	return Mandate{
		ID: uuid.Must(uuid.NewV7()), ReceiverAccount: receiver, ReceiverKey: receiverKey, ReceiverName: receiverName,
		PayerAccount: payer, MaxAmount: max, Frequency: freq, Description: description, Status: MandatePending,
	}, nil
}

// Decide is the payer's answer to a pending mandate.
func (m *Mandate) Decide(payer uuid.UUID, approve bool) error {
	if payer != m.PayerAccount {
		return fmt.Errorf("%w: mandate", ErrNotFound)
	}
	if m.Status != MandatePending {
		return fmt.Errorf("%w: mandate is %s", ErrInvalidArgument, m.Status)
	}
	m.Status = MandateRejected
	if approve {
		m.Status = MandateActive
	}
	return nil
}

// Revoke ends the mandate; either side may do it.
func (m *Mandate) Revoke(by uuid.UUID) error {
	if by != m.PayerAccount && by != m.ReceiverAccount {
		return fmt.Errorf("%w: mandate", ErrNotFound)
	}
	if m.Status != MandateActive && m.Status != MandatePending {
		return fmt.Errorf("%w: mandate is %s", ErrInvalidArgument, m.Status)
	}
	m.Status = MandateRevoked
	return nil
}

// Period identifies the billing period of a date: "2026-11" or "2026-W47".
func (m Mandate) Period(d time.Time) string {
	if m.Frequency == Weekly {
		y, w := d.ISOWeek()
		return fmt.Sprintf("%d-W%02d", y, w)
	}
	return d.Format("2006-01")
}

// Charge checks a charge and records its period: at most one charge per
// period, never above the maximum, only while active.
func (m *Mandate) Charge(by uuid.UUID, amount money.Money, due, today time.Time) error {
	switch {
	case by != m.ReceiverAccount:
		return fmt.Errorf("%w: mandate", ErrNotFound)
	case m.Status != MandateActive:
		return fmt.Errorf("%w: %s", ErrMandateNotActive, m.Status)
	case !amount.IsPositive() || amount.Minor() > m.MaxAmount.Minor():
		return fmt.Errorf("%w: amount above the authorized %s", ErrChargeNotAllowed, m.MaxAmount.Decimal())
	case due.Before(today) || due.Sub(today) > 60*24*time.Hour:
		return fmt.Errorf("%w: due date must be within the next 60 days", ErrInvalidArgument)
	case m.Period(due) == m.LastChargePeriod:
		return fmt.Errorf("%w: already charged for %s", ErrChargeNotAllowed, m.LastChargePeriod)
	}
	m.LastChargePeriod = m.Period(due)
	return nil
}
