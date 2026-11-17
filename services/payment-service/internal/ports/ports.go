// Package ports declares the payment service's dependencies.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/domain"
)

type Store interface {
	// Insert stores p unless its idempotency key exists: then it returns the
	// stored payment and its fingerprint.
	Insert(ctx context.Context, p domain.Payment, fingerprint string) (existing *domain.Payment, existingFingerprint string, err error)
	Payment(ctx context.Context, id uuid.UUID) (domain.Payment, error)
	Payments(ctx context.Context, account uuid.UUID, limit int) ([]domain.Payment, error)
	// Acquire takes a lease on the payment (false if someone else holds it).
	Acquire(ctx context.Context, id uuid.UUID, lease time.Duration) (domain.Payment, bool, error)
	Release(ctx context.Context, id uuid.UUID) error
	Save(ctx context.Context, p domain.Payment, events ...outbox.Message) error
	// Unfinished lists payments in flight not updated since before.
	Unfinished(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error)
	// Due lists scheduled payments whose date has come.
	Due(ctx context.Context, today time.Time, limit int) ([]uuid.UUID, error)
}

// Registry is the boleto registry (CIP's Nova Plataforma de Cobrança):
// who the payee is, and whether the boleto can still be paid.
type Registry interface {
	Lookup(ctx context.Context, b domain.Boleto) (domain.Payee, error)
}

// Clearing settles a payment with the payee's bank. Idempotent on the
// payment id; refusals are domain.RefusedError.
type Clearing interface {
	Settle(ctx context.Context, payment uuid.UUID, b domain.Boleto, amount int64) error
}

type TxResult struct {
	ID            uuid.UUID
	Status        string
	FailureReason string
}

type Transactions interface {
	Reserve(ctx context.Context, key string, account uuid.UUID, amount int64, description string, metadata map[string]string) (TxResult, error)
	Complete(ctx context.Context, id uuid.UUID) (TxResult, error)
	Compensate(ctx context.Context, id uuid.UUID, reason string) (TxResult, error)
}
