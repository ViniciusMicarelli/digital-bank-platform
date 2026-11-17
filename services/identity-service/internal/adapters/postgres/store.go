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
	totp_last_step, failed_attempts, locked_until`

func scanCreds(row pgx.Row) (domain.Credentials, error) {
	var c domain.Credentials
	err := row.Scan(&c.CustomerID, &c.Email, &c.FullName, &c.PasswordHash, &c.TOTPSecretEnc, &c.TOTPEnabled,
		&c.TOTPLastStep, &c.FailedAttempts, &c.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, fmt.Errorf("%w: customer", domain.ErrNotFound)
	}
	return c, err
}

func (s *Store) InsertCredentials(ctx context.Context, c domain.Credentials, ev outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO credentials (customer_id, email, full_name, password_hash) VALUES ($1, $2, $3, $4)`,
			c.CustomerID, c.Email, c.FullName, c.PasswordHash)
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
	_, err := t.tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, now()) WHERE family_id = $1`, family)
	return err
}
