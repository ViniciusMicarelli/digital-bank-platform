// Package ports declares the saga's dependencies.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/domain"
)

// Store persists sagas. Every Save is atomic with its events.
type Store interface {
	// Create inserts t unless the key exists; then it returns the stored one and its fingerprint.
	Create(ctx context.Context, t domain.Transaction) (existing *domain.Transaction, fingerprint []byte, err error)
	Get(ctx context.Context, id uuid.UUID) (domain.Transaction, error)
	// Acquire takes the lease on a saga for d. ok=false if another worker holds it.
	Acquire(ctx context.Context, id uuid.UUID, d time.Duration) (t domain.Transaction, ok bool, err error)
	// Release gives the lease back.
	Release(ctx context.Context, id uuid.UUID) error
	// Save stores the new state, logs the step and writes events, in one transaction.
	Save(ctx context.Context, t domain.Transaction, step Step, events ...outbox.Message) error
	// Unfinished lists sagas that may need the recovery worker: PENDING or
	// COMPENSATING untouched since staleBefore, RESERVED since reservedBefore.
	Unfinished(ctx context.Context, staleBefore, reservedBefore time.Time, limit int) ([]Unfinished, error)
}

// Unfinished identifies a saga the recovery worker may act on.
type Unfinished struct {
	ID        uuid.UUID
	Type      domain.Type
	Status    domain.Status
	UpdatedAt time.Time
}

// Step is an audit record of what a saga step did.
type Step struct {
	Name    string
	Outcome string // OK, REJECTED or ERROR
	Detail  string
}

// Ledger operations used by the saga. All are idempotent.
// Business refusals are returned as *domain.RejectedError.
type Ledger interface {
	Transfer(ctx context.Context, key string, from, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (entryID uuid.UUID, err error)
	Hold(ctx context.Context, key string, from, to uuid.UUID, amount money.Money, description string) (holdID uuid.UUID, err error)
	// Capture posts the hold; amount (optional) captures less than was held.
	Capture(ctx context.Context, holdID uuid.UUID, amount *money.Money) (entryID uuid.UUID, err error)
	Release(ctx context.Context, holdID uuid.UUID) error
}

// Accounts operations used by the saga.
type Accounts interface {
	ConsumeLimit(ctx context.Context, account uuid.UUID, txID string, amount money.Money) error
	RestoreLimit(ctx context.Context, account uuid.UUID, txID string) error
	// EnsureActive rejects when the account does not exist or is not active.
	EnsureActive(ctx context.Context, account uuid.UUID) error
}
