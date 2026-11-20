// Package postgres implements the identity store.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/ports"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

func (s *Store) InTx(ctx context.Context, fn func(ports.Tx) error) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error { return fn(&txStore{tx: tx}) })
}

const credCols = `customer_id, email, full_name, password_hash, totp_secret_enc, totp_enabled,
	totp_last_step, failed_attempts, locked_until, role`

func scanCreds(row pgx.Row) (domain.Credentials, error) {
	var c domain.Credentials
	err := row.Scan(&c.CustomerID, &c.Email, &c.FullName, &c.PasswordHash, &c.TOTPSecretEnc, &c.TOTPEnabled,
		&c.TOTPLastStep, &c.FailedAttempts, &c.LockedUntil, &c.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, fmt.Errorf("%w: customer", domain.ErrNotFound)
	}
	return c, err
}

func (s *Store) InsertCredentials(ctx context.Context, c domain.Credentials, ev outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		role := c.Role
		if role == "" {
			role = domain.RoleCustomer
		}
		_, err := tx.Exec(ctx, `INSERT INTO credentials (customer_id, email, full_name, password_hash, role) VALUES ($1, $2, $3, $4, $5)`,
			c.CustomerID, c.Email, c.FullName, c.PasswordHash, role)
		if pg.IsUniqueViolation(err, "credentials_email_key") {
			return domain.ErrEmailTaken
		}
		if err != nil {
			return err
		}
		return outbox.Insert(ctx, tx, ev)
	})
}

func (s *Store) Credentials(ctx context.Context, id uuid.UUID) (domain.Credentials, error) {
	return scanCreds(s.pool.QueryRow(ctx, `SELECT `+credCols+` FROM credentials WHERE customer_id = $1`, id))
}

const keyCols = `kid, private_key_enc, public_key, created_at`

func scanKey(row pgx.Row) (ports.StoredKey, error) {
	var k ports.StoredKey
	err := row.Scan(&k.Kid, &k.PrivateKeyEnc, &k.PublicKey, &k.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return k, fmt.Errorf("%w: signing key", domain.ErrNotFound)
	}
	return k, err
}

func (s *Store) ActiveKey(ctx context.Context) (ports.StoredKey, error) {
	return scanKey(s.pool.QueryRow(ctx, `SELECT `+keyCols+` FROM signing_keys WHERE retired_at IS NULL`))
}

func (s *Store) PublishedKeys(ctx context.Context, now time.Time) ([]ports.StoredKey, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+keyCols+` FROM signing_keys
		WHERE expires_at IS NULL OR expires_at > $1 ORDER BY created_at DESC`, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ports.StoredKey, error) { return scanKey(r) })
}

func (s *Store) InsertFirstKey(ctx context.Context, k ports.StoredKey) (bool, error) {
	// The partial unique index allows one active key; a concurrent first
	// insert loses and simply uses the winner's key.
	tag, err := s.pool.Exec(ctx, `INSERT INTO signing_keys (kid, private_key_enc, public_key)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, k.Kid, k.PrivateKeyEnc, k.PublicKey)
	return tag.RowsAffected() == 1, err
}

func (s *Store) RotateKey(ctx context.Context, k ports.StoredKey, grace time.Duration) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE signing_keys SET retired_at = now(), expires_at = now() + $1::INTERVAL
			WHERE retired_at IS NULL`, grace.String()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO signing_keys (kid, private_key_enc, public_key) VALUES ($1, $2, $3)`,
			k.Kid, k.PrivateKeyEnc, k.PublicKey)
		return err
	})
}

type txStore struct{ tx pgx.Tx }

func (t *txStore) CredentialsByEmailForUpdate(ctx context.Context, email string) (domain.Credentials, error) {
	return scanCreds(t.tx.QueryRow(ctx, `SELECT `+credCols+` FROM credentials WHERE email = $1 FOR UPDATE`, email))
}

func (t *txStore) CredentialsForUpdate(ctx context.Context, id uuid.UUID) (domain.Credentials, error) {
	return scanCreds(t.tx.QueryRow(ctx, `SELECT `+credCols+` FROM credentials WHERE customer_id = $1 FOR UPDATE`, id))
}

func (t *txStore) SaveCredentials(ctx context.Context, c domain.Credentials) error {
	_, err := t.tx.Exec(ctx, `UPDATE credentials
		SET totp_secret_enc = $2, totp_enabled = $3, totp_last_step = $4, failed_attempts = $5, locked_until = $6, updated_at = now()
		WHERE customer_id = $1`, c.CustomerID, c.TOTPSecretEnc, c.TOTPEnabled, c.TOTPLastStep, c.FailedAttempts, c.LockedUntil)
	return err
}

func (t *txStore) InsertRefreshToken(ctx context.Context, r ports.RefreshToken) error {
	_, err := t.tx.Exec(ctx, `INSERT INTO refresh_tokens (id, family_id, customer_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, r.ID, r.FamilyID, r.CustomerID, r.Hash, r.ExpiresAt)
	return err
}

func (t *txStore) RefreshTokenForUpdate(ctx context.Context, hash []byte) (ports.RefreshToken, error) {
	var r ports.RefreshToken
	err := t.tx.QueryRow(ctx, `SELECT id, family_id, customer_id, token_hash, expires_at, revoked_at
		FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE`, hash).
		Scan(&r.ID, &r.FamilyID, &r.CustomerID, &r.Hash, &r.ExpiresAt, &r.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, fmt.Errorf("%w: refresh token", domain.ErrNotFound)
	}
	return r, err
}

func (t *txStore) RevokeRefreshToken(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) error {
	_, err := t.tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2 WHERE id = $1`, id, replacedBy)
	return err
}

func (t *txStore) RevokeFamily(ctx context.Context, family uuid.UUID) error {
	if _, err := t.tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, now()) WHERE family_id = $1`, family); err != nil {
		return err
	}
	_, err := t.tx.Exec(ctx, `UPDATE sessions SET revoked_at = COALESCE(revoked_at, now()) WHERE id = $1`, family)
	return err
}

func (t *txStore) InsertSession(ctx context.Context, s domain.Session) error {
	_, err := t.tx.Exec(ctx, `INSERT INTO sessions (id, customer_id, device, user_agent, ip, created_at, last_used_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, s.ID, s.CustomerID, s.Device, s.UserAgent, s.IP, s.CreatedAt, s.LastUsedAt)
	return err
}

func (t *txStore) TouchSession(ctx context.Context, id uuid.UUID, ip string, now time.Time) error {
	_, err := t.tx.Exec(ctx, `UPDATE sessions SET last_used_at = $2, ip = CASE WHEN $3 = '' THEN ip ELSE $3 END WHERE id = $1`, id, now, ip)
	return err
}

func (t *txStore) SeenDevice(ctx context.Context, customer uuid.UUID, userAgent string) (anySession, same bool, err error) {
	err = t.tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sessions WHERE customer_id = $1),
		EXISTS (SELECT 1 FROM sessions WHERE customer_id = $1 AND user_agent = $2)`, customer, userAgent).Scan(&anySession, &same)
	return anySession, same, err
}

func (t *txStore) SessionOwner(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	var owner uuid.UUID
	err := t.tx.QueryRow(ctx, `SELECT customer_id FROM sessions WHERE id = $1 AND revoked_at IS NULL FOR UPDATE`, id).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return owner, fmt.Errorf("%w: session", domain.ErrNotFound)
	}
	return owner, err
}

func (t *txStore) OtherSessions(ctx context.Context, customer, keep uuid.UUID) ([]uuid.UUID, error) {
	rows, err := t.tx.Query(ctx, `SELECT id FROM sessions WHERE customer_id = $1 AND id <> $2 AND revoked_at IS NULL FOR UPDATE`, customer, keep)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (t *txStore) Publish(ctx context.Context, m outbox.Message) error {
	return outbox.Insert(ctx, t.tx, m)
}

func (s *Store) Sessions(ctx context.Context, customer uuid.UUID, since time.Time) ([]domain.Session, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, customer_id, device, user_agent, ip, created_at, last_used_at FROM sessions
		WHERE customer_id = $1 AND revoked_at IS NULL AND last_used_at > $2 ORDER BY last_used_at DESC, id DESC LIMIT 50`, customer, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Session, error) {
		var x domain.Session
		return x, r.Scan(&x.ID, &x.CustomerID, &x.Device, &x.UserAgent, &x.IP, &x.CreatedAt, &x.LastUsedAt)
	})
}

// SetRole changes the role of a user (back-office bootstrap).
func (s *Store) SetRole(ctx context.Context, id uuid.UUID, role domain.Role) error {
	_, err := s.pool.Exec(ctx, `UPDATE credentials SET role = $2, updated_at = now() WHERE customer_id = $1`, id, role)
	return err
}

// CredentialsByEmail reads a user by e-mail without locking.
func (s *Store) CredentialsByEmail(ctx context.Context, email string) (domain.Credentials, error) {
	return scanCreds(s.pool.QueryRow(ctx, `SELECT `+credCols+` FROM credentials WHERE email = $1`, email))
}
