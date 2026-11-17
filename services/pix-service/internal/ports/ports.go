// Package ports declares the Pix service's dependencies.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
)

type Store interface {
	// InsertKey stores k if the value is free and the account has room.
	InsertKey(ctx context.Context, k domain.Key, event outbox.Message) error
	DeleteKey(ctx context.Context, account, keyID uuid.UUID, event func(domain.Key) (outbox.Message, error)) error
	Keys(ctx context.Context, account uuid.UUID) ([]domain.Key, error)
	KeyByValue(ctx context.Context, value string) (domain.Key, error)

	// CreateTransfer inserts t unless its idempotency key (or E2E id) exists;
	// then it returns the stored transfer and its fingerprint.
	CreateTransfer(ctx context.Context, t domain.Transfer) (existing *domain.Transfer, fingerprint []byte, err error)
	Transfer(ctx context.Context, id uuid.UUID) (domain.Transfer, error)
	Acquire(ctx context.Context, id uuid.UUID, d time.Duration) (domain.Transfer, bool, error)
	Release(ctx context.Context, id uuid.UUID) error
	SaveTransfer(ctx context.Context, t domain.Transfer, events ...outbox.Message) error
	Unfinished(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error)
}

// Holder is the owner of one of our accounts.
type Holder struct {
	Name     string
	Document string
	Active   bool
}

type Accounts interface {
	Holder(ctx context.Context, account uuid.UUID) (Holder, error)
}

// TxResult is the outcome of a transaction-service call.
type TxResult struct {
	ID            uuid.UUID
	Status        string // PENDING, RESERVED, COMPLETED, COMPENSATING, FAILED
	FailureReason string
}

type Transactions interface {
	Transfer(ctx context.Context, key string, from, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (TxResult, error)
	ReserveOutgoing(ctx context.Context, key string, from uuid.UUID, amount money.Money, description string, metadata map[string]string) (TxResult, error)
	Complete(ctx context.Context, id uuid.UUID) (TxResult, error)
	Compensate(ctx context.Context, id uuid.UUID, reason string) (TxResult, error)
	CreditIncoming(ctx context.Context, key string, to uuid.UUID, amount money.Money, description string, metadata map[string]string) (TxResult, error)
}

// Directory is the Central Bank's key directory (DICT) for keys held at other banks.
type Directory interface {
	Lookup(ctx context.Context, key string) (domain.Party, error)
}

// SPI is the Central Bank's settlement system. Send is idempotent on the E2E id
// and returns *domain.RejectedError when the receiving bank refuses.
type SPI interface {
	Send(ctx context.Context, endToEndID string, receiver domain.Party, key string, amount money.Money) error
}
