// Package ports declares what the account application needs from outside.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/domain"
)

// Store persists accounts and limits.
type Store interface {
	InTx(ctx context.Context, fn func(Tx) error) error
	Account(ctx context.Context, id uuid.UUID) (domain.Account, error)
	AccountsByCustomer(ctx context.Context, customerID string) ([]domain.Account, error)
	Limits(ctx context.Context, id uuid.UUID) (domain.Limits, error)
	Usage(ctx context.Context, id uuid.UUID, localDate time.Time, nightOf *time.Time) (domain.Usage, error)
}

// Tx is a unit of work.
type Tx interface {
	// InsertAccount assigns the account number and stores the account with
	// default limits. If the idempotency key exists it returns the stored
	// account and created=false.
	InsertAccount(ctx context.Context, a domain.Account, limits domain.Limits) (stored domain.Account, created bool, err error)
	AccountForUpdate(ctx context.Context, id uuid.UUID) (domain.Account, error)
	UpdateAccount(ctx context.Context, a domain.Account) error

	LimitsForUpdate(ctx context.Context, id uuid.UUID) (domain.Limits, error)
	SaveLimits(ctx context.Context, id uuid.UUID, l domain.Limits) error
	Usage(ctx context.Context, id uuid.UUID, localDate time.Time, nightOf *time.Time) (domain.Usage, error)
	// RecordUsage stores a usage row; recorded=false if the transaction id was already recorded.
	RecordUsage(ctx context.Context, id uuid.UUID, txID string, amount int64, localDate time.Time, nightOf *time.Time) (recorded bool, err error)
	// RestoreUsage marks a transaction's usage as given back.
	RestoreUsage(ctx context.Context, id uuid.UUID, txID string) error

	Publish(ctx context.Context, msg outbox.Message) error
}

// Balance as reported by the ledger.
type Balance struct {
	Posted, Pending, Available money.Money
}

// Ledger is the ledger-service, seen from accounts.
type Ledger interface {
	// CreateCustomerAccount is idempotent on id.
	CreateCustomerAccount(ctx context.Context, id uuid.UUID, currency money.Currency) error
	Balance(ctx context.Context, id uuid.UUID) (Balance, error)
}
