// Package domain holds the Pix rules: keys, end-to-end IDs and the
// transfer state machine.
package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/document"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

var (
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrNotFound            = errors.New("not found")
	ErrKeyTaken            = errors.New("pix key already registered")
	ErrTooManyKeys         = errors.New("account already has the maximum number of keys")
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	ErrBusy                = errors.New("transfer is being processed")
)

// RejectedError is a business refusal from a downstream service.
type RejectedError struct{ Reason, Msg string }

func (e *RejectedError) Error() string { return e.Reason + ": " + e.Msg }

func Reject(reason, msg string) error { return &RejectedError{Reason: reason, Msg: msg} }

func AsRejected(err error) (*RejectedError, bool) {
	var r *RejectedError
	return r, errors.As(err, &r)
}

// Bank identification of this institution in the Pix system.
const (
	OurISPB     = "99999999"
	OurBankName = "Digital Bank"
	// MaxKeysPerAccount is the Central Bank limit for individuals.
	MaxKeysPerAccount = 5
)

type KeyType string

const (
	KeyCPF   KeyType = "CPF"
	KeyEmail KeyType = "EMAIL"
	KeyPhone KeyType = "PHONE"
	KeyEVP   KeyType = "EVP"
)

// Key is a Pix key owned by one of our accounts.
type Key struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	Type      KeyType
	Value     string
	CreatedAt time.Time
}

var phoneRE = regexp.MustCompile(`^\+55[1-9][0-9]9[0-9]{8}$`)

// NormalizeKey validates value for its type and returns the canonical form:
// CPF digits, lower-case email, E.164 phone (+55DD9XXXXXXXX), lower-case UUID.
func NormalizeKey(t KeyType, value string) (string, error) {
	value = strings.TrimSpace(value)
	invalid := func(what string) error { return fmt.Errorf("%w: invalid %s key", ErrInvalidArgument, what) }
	switch t {
	case KeyCPF:
		cpf, err := document.NormalizeCPF(value)
		if err != nil {
			return "", invalid("CPF")
		}
		return cpf, nil
	case KeyEmail:
		addr, err := mail.ParseAddress(value)
		if err != nil || addr.Address != value || len(value) > 77 || !strings.Contains(value[strings.LastIndex(value, "@"):], ".") {
			return "", invalid("email")
		}
		return strings.ToLower(value), nil
	case KeyPhone:
		phone := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(value)
		if !phoneRE.MatchString(phone) {
			return "", invalid("phone")
		}
		return phone, nil
	case KeyEVP:
		id, err := uuid.Parse(value)
		if err != nil {
			return "", invalid("random")
		}
		return id.String(), nil
	}
	return "", fmt.Errorf("%w: unknown key type %q", ErrInvalidArgument, t)
}

// DetectKeyType guesses the type of a key typed by a customer.
func DetectKeyType(value string) (KeyType, string, error) {
	for _, t := range []KeyType{KeyEVP, KeyEmail, KeyPhone, KeyCPF} {
		if v, err := NormalizeKey(t, value); err == nil {
			return t, v, nil
		}
	}
	return "", "", fmt.Errorf("%w: unrecognized Pix key", ErrInvalidArgument)
}

// NewKey builds a key for an account. EVP keys get a fresh random UUID.
func NewKey(account uuid.UUID, t KeyType, value string) (Key, error) {
	if t == KeyEVP {
		value = uuid.NewString()
	}
	v, err := NormalizeKey(t, value)
	if err != nil {
		return Key{}, err
	}
	return Key{ID: uuid.Must(uuid.NewV7()), AccountID: account, Type: t, Value: v}, nil
}

// Party is a Pix participant as displayed to customers.
type Party struct {
	Name           string
	DocumentMasked string
	ISPB           string
	BankName       string
}

// NewEndToEndID builds a Pix E2E id: "E" + ISPB(8) + yyyyMMddHHmm (UTC) + 11 random alphanumerics.
func NewEndToEndID(ispb string, at time.Time) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 11)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return "E" + ispb + at.UTC().Format("200601021504") + string(b)
}

var e2eRE = regexp.MustCompile(`^E[0-9]{8}[0-9]{12}[a-zA-Z0-9]{11}$`)

// ValidEndToEndID checks the E2E id format.
func ValidEndToEndID(s string) bool { return e2eRE.MatchString(s) }

type Direction string

const (
	Outgoing Direction = "OUTGOING"
	Incoming Direction = "INCOMING"
)

type Status string

const (
	StatusCreated       Status = "CREATED"
	StatusFundsReserved Status = "FUNDS_RESERVED"
	StatusSentToSPI     Status = "SENT_TO_SPI"
	StatusCompleted     Status = "COMPLETED"
	StatusFailed        Status = "FAILED"
	StatusRejected      Status = "REJECTED"
)

func (s Status) Terminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusRejected
}

// Transfer is a Pix payment, sent or received.
type Transfer struct {
	ID                uuid.UUID
	IdempotencyKey    string
	EndToEndID        string
	Direction         Direction
	Status            Status
	AccountID         uuid.UUID // our customer
	Key               string    // receiver key (outgoing) or our key (incoming)
	Counterparty      Party
	InternalAccountID uuid.UUID // counterparty account when it is one of ours
	Amount            money.Money
	Description       string
	TransactionID     uuid.UUID
	FailureReason     string
	QRTxID            string    // charge paid by this transfer, if any
	RefundOf          uuid.UUID // incoming transfer this one refunds, if any
	Refunded          int64     // incoming: minor units already refunded or reserved
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Refund rules (Central Bank: a received Pix may be returned within 90 days).
const RefundWindow = 90 * 24 * time.Hour

var (
	ErrNotRefundable  = errors.New("transfer cannot be refunded")
	ErrRefundTooLarge = errors.New("refund exceeds what is left to refund")
	ErrQRUnavailable  = errors.New("QR code expired or already paid")
)

// NewRefund builds the outgoing transfer that returns (part of) a received Pix
// to the payer. It goes through the same reserve -> SPI -> capture flow.
func NewRefund(idemKey string, original Transfer, amount money.Money, reason string, now time.Time) (Transfer, error) {
	switch {
	case idemKey == "":
		return Transfer{}, fmt.Errorf("%w: idempotency key is required", ErrInvalidArgument)
	case original.Direction != Incoming || original.Status != StatusCompleted:
		return Transfer{}, fmt.Errorf("%w: only completed incoming transfers can be refunded", ErrNotRefundable)
	case now.Sub(original.CreatedAt) > RefundWindow:
		return Transfer{}, fmt.Errorf("%w: older than 90 days", ErrNotRefundable)
	case !amount.IsPositive() || amount.Currency() != money.BRL:
		return Transfer{}, fmt.Errorf("%w: refund amount must be positive BRL", ErrInvalidArgument)
	case amount.Minor() > original.Amount.Minor()-original.Refunded:
		return Transfer{}, fmt.Errorf("%w: %d left", ErrRefundTooLarge, original.Amount.Minor()-original.Refunded)
	case len(reason) > 140:
		return Transfer{}, fmt.Errorf("%w: reason longer than 140 characters", ErrInvalidArgument)
	}
	if reason == "" {
		reason = "Devolução Pix"
	}
	return Transfer{
		ID:             uuid.Must(uuid.NewV7()),
		IdempotencyKey: idemKey,
		EndToEndID:     NewEndToEndID(OurISPB, now),
		Direction:      Outgoing,
		Status:         StatusCreated,
		AccountID:      original.AccountID,
		Key:            "",
		Counterparty:   original.Counterparty,
		Amount:         amount,
		Description:    reason,
		RefundOf:       original.ID,
	}, nil
}

// QRCode is a charge issued by one of our customers.
type QRCode struct {
	TxID           string
	AccountID      uuid.UUID
	Key            string
	Amount         *money.Money
	Description    string
	OneTime        bool
	ExpiresAt      *time.Time
	PaidTransferID uuid.UUID
	CreatedAt      time.Time
}

// NewQRTxID returns a random 25-character alphanumeric transaction id.
func NewQRTxID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 25)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// Payable checks a charge can be paid now by transfer id (re-checks by the
// same transfer are fine, so retries work).
func (q QRCode) Payable(now time.Time, transfer uuid.UUID) error {
	if q.ExpiresAt != nil && !now.Before(*q.ExpiresAt) {
		return fmt.Errorf("%w: expired", ErrQRUnavailable)
	}
	if q.OneTime && q.PaidTransferID != uuid.Nil && q.PaidTransferID != transfer {
		return fmt.Errorf("%w: already paid", ErrQRUnavailable)
	}
	return nil
}

// Internal reports whether both sides are accounts of this bank.
func (t Transfer) Internal() bool { return t.InternalAccountID != uuid.Nil }

// NewOutgoing validates a send request.
func NewOutgoing(idemKey string, from uuid.UUID, key string, receiver Party, receiverAccount uuid.UUID, amount money.Money, description string, now time.Time) (Transfer, error) {
	switch {
	case idemKey == "":
		return Transfer{}, fmt.Errorf("%w: idempotency key is required", ErrInvalidArgument)
	case from == uuid.Nil:
		return Transfer{}, fmt.Errorf("%w: from account is required", ErrInvalidArgument)
	case !amount.IsPositive():
		return Transfer{}, fmt.Errorf("%w: amount must be positive", ErrInvalidArgument)
	case amount.Currency() != money.BRL:
		return Transfer{}, fmt.Errorf("%w: Pix only moves BRL", ErrInvalidArgument)
	case len(description) > 140:
		return Transfer{}, fmt.Errorf("%w: description longer than 140 characters", ErrInvalidArgument)
	case receiverAccount == from:
		return Transfer{}, fmt.Errorf("%w: cannot send a Pix to the same account", ErrInvalidArgument)
	}
	return Transfer{
		ID:                uuid.Must(uuid.NewV7()),
		IdempotencyKey:    idemKey,
		EndToEndID:        NewEndToEndID(OurISPB, now),
		Direction:         Outgoing,
		Status:            StatusCreated,
		AccountID:         from,
		Key:               key,
		Counterparty:      receiver,
		InternalAccountID: receiverAccount,
		Amount:            amount,
		Description:       description,
	}, nil
}

// Fingerprint identifies a send request for idempotency checks.
func (t Transfer) Fingerprint() []byte {
	h := sha256.New()
	h.Write([]byte(t.Direction))
	h.Write(t.AccountID[:])
	h.Write([]byte(t.Key))
	h.Write([]byte{0})
	_ = binary.Write(h, binary.BigEndian, t.Amount.Minor())
	h.Write([]byte(t.Description))
	if t.QRTxID != "" {
		h.Write([]byte("qr:" + t.QRTxID))
	}
	if t.RefundOf != uuid.Nil {
		h.Write(append([]byte("refund:"), t.RefundOf[:]...))
	}
	return h.Sum(nil)
}
