// Package ports declares what the savings application needs.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/domain"
)

type Store interface {
	InTx(ctx context.Context, fn func(Tx) error) error
	BoxByKey(ctx context.Context, key string) (*domain.Box, error)
	Box(ctx context.Context, id uuid.UUID) (domain.Box, error)
	Boxes(ctx context.Context, account uuid.UUID) ([]domain.Box, error)
	Movement(ctx context.Context, id uuid.UUID) (domain.Movement, error)
	Movements(ctx context.Context, box uuid.UUID, limit int) ([]domain.Movement, error)
	PendingMovements(ctx context.Context, before time.Time, limit int) ([]domain.Movement, error)
	// BoxesToAccrue lists active boxes whose yield was last paid before day.
	BoxesToAccrue(ctx context.Context, day time.Time, limit int) ([]uuid.UUID, error)
	// BoxesScheduled lists active boxes with a monthly deposit due by day.
	BoxesScheduled(ctx context.Context, day time.Time, limit int) ([]uuid.UUID, error)
}

type Tx interface {
	// InsertBox returns false when the key exists (a retry).
	InsertBox(b domain.Box) (bool, error)
	CountActive(account uuid.UUID) (int, error)
	LockBox(id uuid.UUID) (domain.Box, error)
	SaveBox(b domain.Box) error
	// InsertMovement returns the stored movement when (box, key) exists.
	InsertMovement(m domain.Movement) (existing *domain.Movement, err error)
	LockMovement(id uuid.UUID) (domain.Movement, error)
	SaveMovement(m domain.Movement) error
}

// Ledger holds each box as a customer account (no overdraft).
type Ledger interface {
	CreateBoxAccount(ctx context.Context, id uuid.UUID) error
	Balance(ctx context.Context, id uuid.UUID) (int64, error)
	// BalanceAt is the posted balance at an instant.
	BalanceAt(ctx context.Context, id uuid.UUID, at time.Time) (int64, error)
}

// TxResult is the outcome of a one-step transaction-service movement.
type TxResult struct {
	Status        string // COMPLETED or FAILED
	FailureReason string
}

type Transactions interface {
	Post(ctx context.Context, key, typ string, from, to uuid.UUID, amount int64, description string, metadata map[string]string) (TxResult, error)
}
