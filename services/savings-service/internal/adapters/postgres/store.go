// Package postgres stores boxes and their movements.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/ports"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

type unit struct {
	ctx context.Context
	tx  pgx.Tx
}

func (s *Store) InTx(ctx context.Context, fn func(ports.Tx) error) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error { return fn(&unit{ctx: ctx, tx: tx}) })
}

const boxCols = `id, idempotency_key, account_id, name, goal, status, yielded, yield_carry_micro, last_accrued, created_at`

func scanBox(row pgx.Row) (domain.Box, error) {
	var (
		b      domain.Box
		status string
	)
	err := row.Scan(&b.ID, &b.IdempotencyKey, &b.AccountID, &b.Name, &b.Goal, &status, &b.Yielded, &b.YieldCarryMicro, &b.LastAccrued, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, fmt.Errorf("%w: box", domain.ErrNotFound)
	}
	b.Status = domain.Status(status)
	return b, err
}

const movementCols = `id, box_id, idempotency_key, kind, amount, status, failure_reason, created_at`

func scanMovement(row pgx.Row) (domain.Movement, error) {
	var (
		m            domain.Movement
		kind, status string
	)
	err := row.Scan(&m.ID, &m.BoxID, &m.IdempotencyKey, &kind, &m.Amount, &status, &m.FailureReason, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, fmt.Errorf("%w: movement", domain.ErrNotFound)
	}
	m.Kind, m.Status = domain.Kind(kind), domain.MovementStatus(status)
	return m, err
}

func collect[T any](rows pgx.Rows, err error, scan func(pgx.Row) (T, error)) ([]T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) BoxByKey(ctx context.Context, key string) (*domain.Box, error) {
	b, err := scanBox(s.pool.QueryRow(ctx, `SELECT `+boxCols+` FROM boxes WHERE idempotency_key = $1`, key))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return &b, err
}

func (s *Store) Box(ctx context.Context, id uuid.UUID) (domain.Box, error) {
	return scanBox(s.pool.QueryRow(ctx, `SELECT `+boxCols+` FROM boxes WHERE id = $1`, id))
}

func (s *Store) Boxes(ctx context.Context, account uuid.UUID) ([]domain.Box, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+boxCols+` FROM boxes WHERE account_id = $1 AND status = 'ACTIVE' ORDER BY created_at`, account)
	return collect(rows, err, scanBox)
}

func (s *Store) Movement(ctx context.Context, id uuid.UUID) (domain.Movement, error) {
	return scanMovement(s.pool.QueryRow(ctx, `SELECT `+movementCols+` FROM movements WHERE id = $1`, id))
}

func (s *Store) Movements(ctx context.Context, box uuid.UUID, limit int) ([]domain.Movement, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT `+movementCols+` FROM movements WHERE box_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, box, limit)
	return collect(rows, err, scanMovement)
}

func (s *Store) PendingMovements(ctx context.Context, before time.Time, limit int) ([]domain.Movement, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+movementCols+` FROM movements WHERE status = 'PENDING' AND created_at < $1
		ORDER BY updated_at LIMIT $2`, before, limit)
	return collect(rows, err, scanMovement)
}

func (s *Store) BoxesToAccrue(ctx context.Context, day time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM boxes WHERE status = 'ACTIVE' AND last_accrued < $1 ORDER BY last_accrued LIMIT $2`, day, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (u *unit) InsertBox(b domain.Box) (bool, error) {
	tag, err := u.tx.Exec(u.ctx, `INSERT INTO boxes (id, idempotency_key, account_id, name, goal, status, last_accrued, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (idempotency_key) DO NOTHING`,
		b.ID, b.IdempotencyKey, b.AccountID, b.Name, b.Goal, b.Status, b.LastAccrued, b.CreatedAt)
	return tag.RowsAffected() == 1, err
}

func (u *unit) CountActive(account uuid.UUID) (int, error) {
	var n int
	err := u.tx.QueryRow(u.ctx, `SELECT count(*) FROM boxes WHERE account_id = $1 AND status = 'ACTIVE'`, account).Scan(&n)
	return n, err
}

func (u *unit) LockBox(id uuid.UUID) (domain.Box, error) {
	return scanBox(u.tx.QueryRow(u.ctx, `SELECT `+boxCols+` FROM boxes WHERE id = $1 FOR UPDATE`, id))
}

func (u *unit) SaveBox(b domain.Box) error {
	_, err := u.tx.Exec(u.ctx, `UPDATE boxes SET name = $2, goal = $3, status = $4, yielded = $5, yield_carry_micro = $6,
		last_accrued = $7, updated_at = now() WHERE id = $1`, b.ID, b.Name, b.Goal, b.Status, b.Yielded, b.YieldCarryMicro, b.LastAccrued)
	return err
}

func (u *unit) InsertMovement(m domain.Movement) (*domain.Movement, error) {
	tag, err := u.tx.Exec(u.ctx, `INSERT INTO movements (id, box_id, idempotency_key, kind, amount, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (box_id, idempotency_key) DO NOTHING`,
		m.ID, m.BoxID, m.IdempotencyKey, m.Kind, m.Amount, m.Status, m.CreatedAt)
	if err != nil || tag.RowsAffected() == 1 {
		return nil, err
	}
	existing, err := scanMovement(u.tx.QueryRow(u.ctx, `SELECT `+movementCols+` FROM movements WHERE box_id = $1 AND idempotency_key = $2`,
		m.BoxID, m.IdempotencyKey))
	return &existing, err
}

func (u *unit) LockMovement(id uuid.UUID) (domain.Movement, error) {
	return scanMovement(u.tx.QueryRow(u.ctx, `SELECT `+movementCols+` FROM movements WHERE id = $1 FOR UPDATE`, id))
}

func (u *unit) SaveMovement(m domain.Movement) error {
	_, err := u.tx.Exec(u.ctx, `UPDATE movements SET status = $2, failure_reason = $3, updated_at = now() WHERE id = $1`,
		m.ID, m.Status, m.FailureReason)
	return err
}
