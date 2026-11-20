// Package ports declares the identity service's storage needs.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/domain"
)

// RefreshToken is a stored refresh token (only its hash).
type RefreshToken struct {
	ID         uuid.UUID
	FamilyID   uuid.UUID
	CustomerID uuid.UUID
	Hash       []byte
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

// StoredKey is a signing key as persisted: the private key is encrypted.
type StoredKey struct {
	Kid           string
	PrivateKeyEnc []byte
	PublicKey     []byte
	CreatedAt     time.Time
}

type Store interface {
	InTx(ctx context.Context, fn func(Tx) error) error
	// InsertCredentials fails with domain.ErrEmailTaken if the email exists.
	InsertCredentials(ctx context.Context, c domain.Credentials, ev outbox.Message) error
	Credentials(ctx context.Context, customerID uuid.UUID) (domain.Credentials, error)
	CredentialsByEmail(ctx context.Context, email string) (domain.Credentials, error)
	SetRole(ctx context.Context, customerID uuid.UUID, role domain.Role) error

	ActiveKey(ctx context.Context) (StoredKey, error)
	// PublishedKeys are the keys whose tokens may still be valid.
	PublishedKeys(ctx context.Context, now time.Time) ([]StoredKey, error)
	// RotateKey makes k the only signing key; the previous one stays
	// published until now + grace.
	RotateKey(ctx context.Context, k StoredKey, grace time.Duration) error
	// InsertFirstKey adds k only if no key signs yet; false if one already does.
	InsertFirstKey(ctx context.Context, k StoredKey) (bool, error)

	// Sessions lists the customer's sessions not revoked and used since since.
	Sessions(ctx context.Context, customer uuid.UUID, since time.Time) ([]domain.Session, error)
}

// Tx is a unit of work; rows read "ForUpdate" stay locked until it ends.
type Tx interface {
	CredentialsByEmailForUpdate(ctx context.Context, email string) (domain.Credentials, error)
	CredentialsForUpdate(ctx context.Context, customerID uuid.UUID) (domain.Credentials, error)
	// SaveCredentials stores the mutable fields: lockout and TOTP state.
	SaveCredentials(ctx context.Context, c domain.Credentials) error

	InsertRefreshToken(ctx context.Context, t RefreshToken) error
	RefreshTokenForUpdate(ctx context.Context, hash []byte) (RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) error
	// RevokeFamily ends a login: its refresh tokens and its session.
	RevokeFamily(ctx context.Context, familyID uuid.UUID) error

	InsertSession(ctx context.Context, s domain.Session) error
	// TouchSession records a refresh: when, and from which address.
	TouchSession(ctx context.Context, id uuid.UUID, ip string, now time.Time) error
	// SeenDevice reports whether the customer logged in before at all, and
	// from this User-Agent.
	SeenDevice(ctx context.Context, customer uuid.UUID, userAgent string) (any, same bool, err error)
	// SessionOwner is the customer of a session not revoked yet (ErrNotFound otherwise).
	SessionOwner(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	// OtherSessions lists the customer's sessions not revoked, except keep.
	OtherSessions(ctx context.Context, customer, keep uuid.UUID) ([]uuid.UUID, error)
	Publish(ctx context.Context, m outbox.Message) error
}
