// Package postgres implements the account ports on PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/secretbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/ports"
)

// crypt encrypts the CPF at rest: AES-256-GCM bound to the account id, plus a
// keyed blind index so the column stays searchable.
type crypt struct {
	box      *secretbox.Box
	indexKey []byte
}

func documentAAD(id uuid.UUID) []byte { return []byte("account-document:" + id.String()) }

func (c crypt) seal(id uuid.UUID, document string) (enc, index []byte) {
	return c.box.Seal([]byte(document), documentAAD(id)), secretbox.BlindIndex(c.indexKey, document)
}

type Store struct {
	pool *pgxpool.Pool
	crypt
}

// NewStore needs the master key-encryption key (32 bytes).
func NewStore(pool *pgxpool.Pool, kek []byte) (*Store, error) {
	box, err := secretbox.New(kek)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, crypt: crypt{box: box, indexKey: secretbox.DeriveKey(kek, "account-document-index-v1")}}, nil
}

var _ ports.Store = (*Store)(nil)

func (s *Store) InTx(ctx context.Context, fn func(ports.Tx) error) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error { return fn(&txStore{tx: tx, crypt: s.crypt}) })
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const accountCols = `id, idempotency_key, customer_id, holder_name, document, document_enc, branch, number,
	currency, status, status_reason, opened_at, closed_at`

func (c crypt) scanAccount(row pgx.Row) (domain.Account, error) {
	var (
		a                domain.Account
		currency, status string
		plain            *string
		enc              []byte
	)
	err := row.Scan(&a.ID, &a.IdempotencyKey, &a.CustomerID, &a.HolderName, &plain, &enc, &a.Branch,
		&a.Number, &currency, &status, &a.StatusReason, &a.OpenedAt, &a.ClosedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, fmt.Errorf("%w: account", domain.ErrNotFound)
	}
	if err != nil {
		return domain.Account{}, err
	}
	a.Currency, a.Status = money.Currency(currency), domain.Status(status)
	switch {
	case enc != nil:
		doc, err := c.box.Open(enc, documentAAD(a.ID))
		if err != nil {
			return domain.Account{}, fmt.Errorf("decrypt document of %s: %w", a.ID, err)
		}
		a.Document = string(doc)
	case plain != nil: // legacy row not yet encrypted
		a.Document = *plain
	}
	return a, nil
}

func (s *Store) Account(ctx context.Context, id uuid.UUID) (domain.Account, error) {
	return s.scanAccount(s.pool.QueryRow(ctx, `SELECT `+accountCols+` FROM accounts WHERE id = $1`, id))
}

func (s *Store) AccountsByCustomer(ctx context.Context, customerID string) ([]domain.Account, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+accountCols+` FROM accounts WHERE customer_id = $1 ORDER BY opened_at`, customerID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Account, error) { return s.scanAccount(r) })
}

// EncryptLegacyDocuments encrypts CPFs stored before migration 0002 and
// clears the plaintext, in batches. Safe to run on every replica at once.
func (s *Store) EncryptLegacyDocuments(ctx context.Context) (int, error) {
	total := 0
	for {
		n := 0
		err := pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
			n = 0
			rows, err := tx.Query(ctx, `SELECT id, document FROM accounts
				WHERE document IS NOT NULL LIMIT 500 FOR UPDATE SKIP LOCKED`)
			if err != nil {
				return err
			}
			type legacy struct {
				id  uuid.UUID
				doc string
			}
			var batch []legacy
			for rows.Next() {
				var l legacy
				if err := rows.Scan(&l.id, &l.doc); err != nil {
					return err
				}
				batch = append(batch, l)
			}
			if err := rows.Err(); err != nil {
				return err
			}
			for _, l := range batch {
				enc, index := s.seal(l.id, l.doc)
				if _, err := tx.Exec(ctx, `UPDATE accounts SET document_enc = $2, document_index = $3, document = NULL WHERE id = $1`,
					l.id, enc, index); err != nil {
					return err
				}
			}
			n = len(batch)
			return nil
		})
		if err != nil || n == 0 {
			return total, err
		}
		total += n
	}
}

func (s *Store) Limits(ctx context.Context, id uuid.UUID) (domain.Limits, error) {
	return scanLimits(s.pool.QueryRow(ctx, `SELECT per_transaction, daily, nightly FROM account_limits WHERE account_id = $1`, id))
}

func (s *Store) Usage(ctx context.Context, id uuid.UUID, day time.Time, night *time.Time) (domain.Usage, error) {
	return usage(ctx, s.pool, id, day, night)
}

func scanLimits(row pgx.Row) (domain.Limits, error) {
	var l domain.Limits
	err := row.Scan(&l.PerTransaction, &l.Daily, &l.Nightly)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, fmt.Errorf("%w: account", domain.ErrNotFound)
	}
	return l, err
}

func usage(ctx context.Context, q querier, id uuid.UUID, day time.Time, night *time.Time) (domain.Usage, error) {
	var u domain.Usage
	err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount) FILTER (WHERE local_date = $2), 0)::BIGINT,
		       COALESCE(SUM(amount) FILTER (WHERE $3::DATE IS NOT NULL AND night_of = $3), 0)::BIGINT
		  FROM limit_usage
		 WHERE account_id = $1 AND NOT restored AND (local_date = $2 OR night_of = $3)`,
		id, day, night).Scan(&u.Daily, &u.Nightly)
	return u, err
}

type txStore struct {
	tx pgx.Tx
	crypt
}

func (t *txStore) InsertAccount(ctx context.Context, a domain.Account, l domain.Limits) (domain.Account, bool, error) {
	var seq int64
	if err := t.tx.QueryRow(ctx, `SELECT nextval('account_number_seq')`).Scan(&seq); err != nil {
		return domain.Account{}, false, err
	}
	a.Number = domain.FormatNumber(seq)
	enc, index := t.seal(a.ID, a.Document)
	tag, err := t.tx.Exec(ctx, `
		INSERT INTO accounts (id, idempotency_key, customer_id, holder_name, document_enc, document_index, branch, number, currency, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) ON CONFLICT (idempotency_key) DO NOTHING`,
		a.ID, a.IdempotencyKey, a.CustomerID, a.HolderName, enc, index, a.Branch, a.Number, a.Currency, a.Status)
	if err != nil {
		return domain.Account{}, false, err
	}
	created := tag.RowsAffected() == 1
	if created {
		if _, err := t.tx.Exec(ctx, `INSERT INTO account_limits (account_id, per_transaction, daily, nightly) VALUES ($1, $2, $3, $4)`,
			a.ID, l.PerTransaction, l.Daily, l.Nightly); err != nil {
			return domain.Account{}, false, err
		}
	}
	stored, err := t.scanAccount(t.tx.QueryRow(ctx, `SELECT `+accountCols+` FROM accounts WHERE idempotency_key = $1`, a.IdempotencyKey))
	return stored, created, err
}

func (t *txStore) AccountForUpdate(ctx context.Context, id uuid.UUID) (domain.Account, error) {
	return t.scanAccount(t.tx.QueryRow(ctx, `SELECT `+accountCols+` FROM accounts WHERE id = $1 FOR UPDATE`, id))
}

func (t *txStore) UpdateAccount(ctx context.Context, a domain.Account) error {
	_, err := t.tx.Exec(ctx, `UPDATE accounts SET status = $2, status_reason = $3, closed_at = $4, updated_at = now() WHERE id = $1`,
		a.ID, a.Status, a.StatusReason, a.ClosedAt)
	return err
}

func (t *txStore) LimitsForUpdate(ctx context.Context, id uuid.UUID) (domain.Limits, error) {
	return scanLimits(t.tx.QueryRow(ctx, `SELECT per_transaction, daily, nightly FROM account_limits WHERE account_id = $1 FOR UPDATE`, id))
}

func (t *txStore) SaveLimits(ctx context.Context, id uuid.UUID, l domain.Limits) error {
	_, err := t.tx.Exec(ctx, `UPDATE account_limits SET per_transaction = $2, daily = $3, nightly = $4, updated_at = now() WHERE account_id = $1`,
		id, l.PerTransaction, l.Daily, l.Nightly)
	return err
}

func (t *txStore) Usage(ctx context.Context, id uuid.UUID, day time.Time, night *time.Time) (domain.Usage, error) {
	return usage(ctx, t.tx, id, day, night)
}

func (t *txStore) RecordUsage(ctx context.Context, id uuid.UUID, txID string, amount int64, day time.Time, night *time.Time) (bool, error) {
	tag, err := t.tx.Exec(ctx, `
		INSERT INTO limit_usage (account_id, transaction_id, amount, local_date, night_of)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (account_id, transaction_id) DO NOTHING`,
		id, txID, amount, day, night)
	return tag.RowsAffected() == 1, err
}

func (t *txStore) RestoreUsage(ctx context.Context, id uuid.UUID, txID string) error {
	_, err := t.tx.Exec(ctx, `UPDATE limit_usage SET restored = true WHERE account_id = $1 AND transaction_id = $2`, id, txID)
	return err
}

func (t *txStore) Publish(ctx context.Context, msg outbox.Message) error {
	return outbox.Insert(ctx, t.tx, msg)
}
