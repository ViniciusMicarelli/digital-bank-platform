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
	// AuthorizationsByCard lists a card's purchases, newest first.
	AuthorizationsByCard(ctx context.Context, card uuid.UUID, limit int) ([]domain.Authorization, error)

	// InTx runs refunds and disputes in one unit of work.
	InTx(ctx context.Context, fn func(Tx) error) error
	Dispute(ctx context.Context, id uuid.UUID) (domain.Dispute, error)
	// Disputes filters by account and/or status (zero values match all), newest first.
	Disputes(ctx context.Context, account uuid.UUID, status domain.DisputeStatus, limit int) ([]domain.Dispute, error)
	// LatestDisputes returns the newest dispute of each authorization that has one.
	LatestDisputes(ctx context.Context, authorizations []uuid.UUID) (map[uuid.UUID]domain.Dispute, error)
	PendingRefunds(ctx context.Context, before time.Time, limit int) ([]domain.Refund, error)
}

// Tx is a unit of work; Lock* take row locks (authorization before dispute
// before card).
type Tx interface {
	LockAuthorization(id uuid.UUID) (domain.Authorization, error)
	SetRefunded(authorization uuid.UUID, refunded int64) error
	// OpenDispute returns the open dispute of an authorization, if any.
	OpenDispute(authorization uuid.UUID) (*domain.Dispute, error)
	InsertDispute(d domain.Dispute) error
	LockDispute(id uuid.UUID) (domain.Dispute, error)
	SaveDispute(d domain.Dispute) error
	RefundByReference(authorization uuid.UUID, reference string) (*domain.Refund, error)
	InsertRefund(r domain.Refund) error
	LockRefund(id uuid.UUID) (domain.Refund, error)
	SaveRefund(r domain.Refund) error
	LockCard(id uuid.UUID) (domain.Card, error)
	SaveCard(c domain.Card) error
	Publish(m outbox.Message) error
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
	// Refund gives money back for a cleared purchase (idempotent on key):
	// COMPLETED, or FAILED with a reason the next try would not change.
	Refund(ctx context.Context, key string, account uuid.UUID, amount money.Money, description string, metadata map[string]string) (TxResult, error)
}
