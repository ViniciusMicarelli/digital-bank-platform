// Package domain holds the account rules: lifecycle, identity data and limits.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

var (
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrNotFound            = errors.New("not found")
	ErrInvalidTransition   = errors.New("invalid status transition")
	ErrNotActive           = errors.New("account is not active")
	ErrBalanceNotZero      = errors.New("account balance is not zero")
	ErrLimitExceeded       = errors.New("limit exceeded")
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
)

// Branch is the single branch of this digital bank.
const Branch = "0001"

type Status string

const (
	StatusActive  Status = "ACTIVE"
	StatusBlocked Status = "BLOCKED"
	StatusClosed  Status = "CLOSED"
)

// Account is a customer's checking account. Its ID is also its ledger account ID.
type Account struct {
	ID             uuid.UUID
	IdempotencyKey string
	CustomerID     string
	HolderName     string
	Document       string // CPF, digits only
	Branch         string
	Number         string
	Currency       money.Currency
	Status         Status
	StatusReason   string
	OpenedAt       time.Time
	ClosedAt       *time.Time
}

// accountNamespace derives account IDs from idempotency keys, so a retried
// OpenAccount always targets the same ledger account.
var accountNamespace = uuid.MustParse("6f1d7f0e-3c55-4b8e-9a51-5b1f4f6b2a10")

// IDForKey returns the deterministic account ID for an idempotency key.
func IDForKey(key string) uuid.UUID { return uuid.NewSHA1(accountNamespace, []byte(key)) }

// NewAccount validates an opening request. The number is assigned on insert.
func NewAccount(key, customerID, holderName, document string, currency money.Currency) (Account, error) {
	holderName = strings.Join(strings.Fields(holderName), " ")
	switch {
	case key == "":
		return Account{}, fmt.Errorf("%w: idempotency key is required", ErrInvalidArgument)
	case customerID == "":
		return Account{}, fmt.Errorf("%w: customer id is required", ErrInvalidArgument)
	case len(holderName) < 3:
		return Account{}, fmt.Errorf("%w: holder name is required", ErrInvalidArgument)
	}
	if currency == "" {
		currency = money.BRL
	}
	if _, err := currency.Decimals(); err != nil {
		return Account{}, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	cpf, err := NormalizeCPF(document)
	if err != nil {
		return Account{}, err
	}
	return Account{
		ID:             IDForKey(key),
		IdempotencyKey: key,
		CustomerID:     customerID,
		HolderName:     holderName,
		Document:       cpf,
		Branch:         Branch,
		Currency:       currency,
		Status:         StatusActive,
	}, nil
}

// SameRequest reports whether a retried opening matches the stored account.
func (a Account) SameRequest(o Account) bool {
	return a.CustomerID == o.CustomerID && a.HolderName == o.HolderName &&
		a.Document == o.Document && a.Currency == o.Currency
}

// Block moves ACTIVE -> BLOCKED.
func (a *Account) Block(reason string) error {
	if a.Status != StatusActive {
		return fmt.Errorf("%w: cannot block a %s account", ErrInvalidTransition, a.Status)
	}
	a.Status, a.StatusReason = StatusBlocked, reason
	return nil
}

// Unblock moves BLOCKED -> ACTIVE.
func (a *Account) Unblock() error {
	if a.Status != StatusBlocked {
		return fmt.Errorf("%w: cannot unblock a %s account", ErrInvalidTransition, a.Status)
	}
	a.Status, a.StatusReason = StatusActive, ""
	return nil
}

// Close moves ACTIVE -> CLOSED. The caller checks the ledger balance.
func (a *Account) Close(reason string, now time.Time) error {
	if a.Status != StatusActive {
		return fmt.Errorf("%w: cannot close a %s account", ErrInvalidTransition, a.Status)
	}
	a.Status, a.StatusReason, a.ClosedAt = StatusClosed, reason, &now
	return nil
}

// FormatNumber renders a sequence number as "NNNNNNNN-D" with a mod-11 check digit.
func FormatNumber(seq int64) string {
	base := fmt.Sprintf("%08d", seq)
	sum := 0
	for i, r := range base {
		sum += int(r-'0') * (9 - i)
	}
	d := sum % 11
	if d == 10 {
		d = 0
	}
	return fmt.Sprintf("%s-%d", base, d)
}

// NormalizeCPF strips punctuation and validates the two CPF check digits.
func NormalizeCPF(s string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		if r == '.' || r == '-' || r == ' ' {
			return -1
		}
		return 'x'
	}, s)
	invalid := fmt.Errorf("%w: invalid CPF", ErrInvalidArgument)
	if len(digits) != 11 || strings.ContainsRune(digits, 'x') {
		return "", invalid
	}
	if strings.Count(digits, digits[:1]) == 11 { // 000.000.000-00, 111..., etc.
		return "", invalid
	}
	for _, n := range []int{9, 10} {
		sum := 0
		for i := range n {
			sum += int(digits[i]-'0') * (n + 1 - i)
		}
		d := sum * 10 % 11
		if d == 10 {
			d = 0
		}
		if d != int(digits[n]-'0') {
			return "", invalid
		}
	}
	return digits, nil
}
