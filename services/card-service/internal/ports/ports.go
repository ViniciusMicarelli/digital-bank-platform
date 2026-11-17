// Package ports declares the card service's dependencies.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/domain"
)

type Store interface {
	// InsertCard stores the card and its vault entry unless the idempotency
	// key exists; then it returns the stored card.
	InsertCard(ctx context.Context, c domain.Card, panEnc, panIndex []byte, ev outbox.Message) (existing *domain.Card, err error)
	Card(ctx context.Context, id uuid.UUID) (domain.Card, error)
	CardsByAccount(ctx context.Context, account uuid.UUID) ([]domain.Card, error)
	CardByPANIndex(ctx context.Context, index []byte) (domain.Card, error)
	PANEnc(ctx context.Context, cardID uuid.UUID) ([]byte, error)
	SaveCard(ctx context.Context, c domain.Card, events ...outbox.Message) error

	InsertToken(ctx context.Context, t domain.Token, panEnc, panIndex []byte, ev outbox.Message) error
	Tokens(ctx context.Context, card uuid.UUID) ([]domain.Token, error)
	Token(ctx context.Context, id uuid.UUID) (domain.Token, error)
	TokenByPANIndex(ctx context.Context, index []byte) (domain.Token, error)
	SaveToken(ctx context.Context, t domain.Token, ev outbox.Message) error

	// ClaimAuthorization inserts a, unless its network reference exists
	// (network retry); then it returns the stored one.
	ClaimAuthorization(ctx context.Context, a domain.Authorization) (existing *domain.Authorization, err error)
	Authorization(ctx context.Context, id uuid.UUID) (domain.Authorization, error)
	// SaveAuthorization updates status, code, reason, cleared amount and
	// transaction id, only if the stored status is still from.
	SaveAuthorization(ctx context.Context, a domain.Authorization, from domain.AuthStatus, events ...outbox.Message) error
	PendingReversals(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error)
}

type Holder struct {
	Name   string
	Active bool
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
	// Reserve consumes the limit and holds the amount (DEBIT_CARD_PURCHASE).
	Reserve(ctx context.Context, key string, account uuid.UUID, amount money.Money, description string, metadata map[string]string) (TxResult, error)
	Complete(ctx context.Context, id uuid.UUID, amount *money.Money) (TxResult, error)
	Compensate(ctx context.Context, id uuid.UUID, reason string) (TxResult, error)
}
