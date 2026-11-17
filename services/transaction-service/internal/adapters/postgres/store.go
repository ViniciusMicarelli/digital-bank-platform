// Package postgres stores sagas in PostgreSQL.
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
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/ports"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

const cols = `id, idempotency_key, type, status, from_account_id, to_account_id, amount, currency,
	description, metadata, limit_consumed, hold_id, entry_id, failure_reason, created_at, updated_at`

func scan(row pgx.Row) (domain.Transaction, error) {
	var (
		t                     domain.Transaction
		typ, status, currency string
		amount                int64
		holdID, entryID       *uuid.UUID
	)
	err := row.Scan(&t.ID, &t.IdempotencyKey, &typ, &status, &t.From, &t.To, &amount, &currency,
		&t.Description, &t.Metadata, &t.LimitConsumed, &holdID, &entryID, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, fmt.Errorf("%w: transaction", domain.ErrNotFound)
	}
	if err != nil {
		return t, err
	}
	t.Type, t.Status = domain.Type(typ), domain.Status(status)
	if t.Amount, err = money.New(amount, money.Currency(currency)); err != nil {
		return t, err
	}
	if holdID != nil {
		t.HoldID = *holdID
	}
	if entryID != nil {
		t.EntryID = *entryID
	}
	return t, nil
}

func (s *Store) Create(ctx context.Context, t domain.Transaction) (*domain.Transaction, []byte, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO transactions (id, idempotency_key, fingerprint, type, status, from_account_id, to_account_id,
		                          amount, currency, description, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) ON CONFLICT (idempotency_key) DO NOTHING`,
		t.ID, t.IdempotencyKey, t.Fingerprint(), t.Type, t.Status, t.From, t.To,
		t.Amount.Minor(), string(t.Amount.Currency()), t.Description, t.Metadata)
	if err != nil || tag.RowsAffected() == 1 {
		return nil, nil, err
	}
	var fp []byte
	if err := s.pool.QueryRow(ctx, `SELECT fingerprint FROM transactions WHERE idempotency_key = $1`, t.IdempotencyKey).Scan(&fp); err != nil {
		return nil, nil, err
	}
	existing, err := scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM transactions WHERE idempotency_key = $1`, t.IdempotencyKey))
	return &existing, fp, err
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (domain.Transaction, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM transactions WHERE id = $1`, id))
}

func (s *Store) Acquire(ctx context.Context, id uuid.UUID, d time.Duration) (domain.Transaction, bool, error) {
	t, err := scan(s.pool.QueryRow(ctx, `
		UPDATE transactions SET locked_until = now() + $2::INTERVAL
		 WHERE id = $1 AND (locked_until IS NULL OR locked_until < now())
		RETURNING `+cols, id, d.String()))
	if errors.Is(err, domain.ErrNotFound) {
		// Either it does not exist or someone else holds the lease.
		current, getErr := s.Get(ctx, id)
		if getErr != nil {
			return domain.Transaction{}, false, getErr
		}
		return current, false, nil
	}
	return t, err == nil, err
}

func (s *Store) Release(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE transactions SET locked_until = NULL WHERE id = $1`, id)
	return err
}

func (s *Store) Save(ctx context.Context, t domain.Transaction, step ports.Step, events ...outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var holdID, entryID *uuid.UUID
		if t.HoldID != uuid.Nil {
			holdID = &t.HoldID
		}
		if t.EntryID != uuid.Nil {
			entryID = &t.EntryID
		}
		_, err := tx.Exec(ctx, `
			UPDATE transactions
			   SET status = $2, limit_consumed = $3, hold_id = $4, entry_id = $5, failure_reason = $6, amount = $7,
			       updated_at = now()
			 WHERE id = $1`,
			t.ID, t.Status, t.LimitConsumed, holdID, entryID, t.FailureReason, t.Amount.Minor())
		if err != nil {
			return err
		}
		if step.Name != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO saga_steps (transaction_id, step, outcome, detail) VALUES ($1, $2, $3, $4)`,
				t.ID, step.Name, step.Outcome, step.Detail); err != nil {
				return err
			}
		}
		for _, ev := range events {
			if err := outbox.Insert(ctx, tx, ev); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Unfinished(ctx context.Context, staleBefore, reservedBefore time.Time, limit int) ([]ports.Unfinished, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, type, status, updated_at FROM transactions
		 WHERE ((status IN ('PENDING', 'COMPENSATING') AND updated_at < $1)
		     OR (status = 'RESERVED' AND updated_at < $2))
		   AND (locked_until IS NULL OR locked_until < now())
		 ORDER BY updated_at
		 LIMIT $3`, staleBefore, reservedBefore, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ports.Unfinished, error) {
		var u ports.Unfinished
		var typ, status string
		err := r.Scan(&u.ID, &typ, &status, &u.UpdatedAt)
		u.Type, u.Status = domain.Type(typ), domain.Status(status)
		return u, err
	})
}
