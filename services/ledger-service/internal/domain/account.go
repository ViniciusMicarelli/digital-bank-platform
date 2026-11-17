package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

// AccountKind decides whether an account has a balance floor.
type AccountKind string

const (
	// KindCustomer accounts cannot go below -OverdraftLimit.
	KindCustomer AccountKind = "CUSTOMER"
	// KindInternal accounts (settlement, funding, revenue) have no floor.
	KindInternal AccountKind = "INTERNAL"
)

// Well-known internal accounts created by the first migration.
var (
	BankFundingAccountID   = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	PixSettlementAccountID = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	CardClearingAccountID  = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	// CreditReceivablesAccountID goes negative by what customers owe on
	// credit card invoices; CreditRevenueAccountID collects fines and interest.
	CreditReceivablesAccountID = uuid.MustParse("00000000-0000-0000-0000-000000000004")
	CreditRevenueAccountID     = uuid.MustParse("00000000-0000-0000-0000-000000000005")
	BoletoSettlementAccountID  = uuid.MustParse("00000000-0000-0000-0000-000000000006")
)

// Account is a bucket of value in the ledger.
type Account struct {
	ID             uuid.UUID
	Kind           AccountKind
	Currency       money.Currency
	OverdraftLimit int64 // minor units, >= 0
	Code           string
	CreatedAt      time.Time
}

// NewAccount validates the attributes of a new account.
func NewAccount(id uuid.UUID, kind AccountKind, currency money.Currency, overdraftLimit int64, code string) (Account, error) {
	if id == uuid.Nil {
		return Account{}, fmt.Errorf("%w: account id is required", ErrInvalidArgument)
	}
	if kind != KindCustomer && kind != KindInternal {
		return Account{}, fmt.Errorf("%w: unknown account kind %q", ErrInvalidArgument, kind)
	}
	if _, err := currency.Decimals(); err != nil {
		return Account{}, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	if overdraftLimit < 0 {
		return Account{}, fmt.Errorf("%w: overdraft limit must not be negative", ErrInvalidArgument)
	}
	return Account{ID: id, Kind: kind, Currency: currency, OverdraftLimit: overdraftLimit, Code: code}, nil
}

// SameAttributes reports whether a retried creation matches the stored account.
func (a Account) SameAttributes(o Account) bool {
	return a.ID == o.ID && a.Kind == o.Kind && a.Currency == o.Currency &&
		a.OverdraftLimit == o.OverdraftLimit && a.Code == o.Code
}

// Balance is the snapshot of an account's money.
type Balance struct {
	AccountID uuid.UUID
	Posted    int64 // sum of postings
	Pending   int64 // sum of active holds
}

// Available is what the customer can spend: posted minus held.
func (b Balance) Available() int64 { return b.Posted - b.Pending }

// checkFloor returns ErrInsufficientFunds when a customer account's available
// balance is below its overdraft floor.
func checkFloor(acc Account, b Balance) error {
	if acc.Kind == KindCustomer && b.Available() < -acc.OverdraftLimit {
		return fmt.Errorf("%w: account %s", ErrInsufficientFunds, acc.ID)
	}
	return nil
}
