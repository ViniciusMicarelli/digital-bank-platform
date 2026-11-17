// Package ports declares what the application needs from the outside world.
// The Postgres adapter implements these interfaces.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/domain"
)

// Store gives access to ledger data. Writes go through InTx.
type Store interface {
	// InTx runs fn in one database transaction. fn may be retried on
	// serialization failures or deadlocks, so it must not have side effects
	// outside the transaction.
	InTx(ctx context.Context, fn func(Tx) error) error

	Account(ctx context.Context, id uuid.UUID) (domain.Account, error)
	Entry(ctx context.Context, id uuid.UUID) (domain.Entry, error)
	Balance(ctx context.Context, accountID uuid.UUID) (domain.Balance, error)
	// PostedAt is the sum of postings up to and including at.
	PostedAt(ctx context.Context, accountID uuid.UUID, at time.Time) (int64, error)
	// Postings lists an account's postings, newest first, before the cursor
	// (posting id; 0 means from the newest).
	Postings(ctx context.Context, accountID uuid.UUID, before int64, limit int) ([]AccountPosting, error)
}

// AccountPosting is a posting seen from one account's perspective.
type AccountPosting struct {
	ID          int64
	EntryID     uuid.UUID
	Amount      int64
	Currency    string
	Description string
	CreatedAt   time.Time
}

// Tx is a unit of work. Row locks are held until it commits.
type Tx interface {
	// InsertAccount creates the account and its zero balance. If the id
	// exists it returns the stored account and created=false.
	InsertAccount(ctx context.Context, a domain.Account) (stored domain.Account, created bool, err error)
	Accounts(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Account, error)
	// LockBalances locks balance rows with SELECT ... FOR UPDATE, always in
	// ascending id order so concurrent transactions cannot deadlock.
	LockBalances(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*domain.Balance, error)
	SaveBalances(ctx context.Context, balances map[uuid.UUID]*domain.Balance) error

	// ClaimEntry inserts the entry header unless its idempotency key exists.
	// On conflict it returns the existing entry and its fingerprint.
	ClaimEntry(ctx context.Context, e domain.Entry) (existing *domain.Entry, fingerprint []byte, err error)
	InsertPostings(ctx context.Context, e domain.Entry) error
	EntryForUpdate(ctx context.Context, id uuid.UUID) (domain.Entry, error)
	// ReversalOf returns the entry that reverses id, if any.
	ReversalOf(ctx context.Context, id uuid.UUID) (*domain.Entry, error)

	// ClaimHold inserts the hold unless its idempotency key exists.
	ClaimHold(ctx context.Context, h domain.Hold) (existing *domain.Hold, fingerprint []byte, err error)
	HoldForUpdate(ctx context.Context, id uuid.UUID) (domain.Hold, error)
	UpdateHold(ctx context.Context, h domain.Hold) error

	Publish(ctx context.Context, msg outbox.Message) error
}
