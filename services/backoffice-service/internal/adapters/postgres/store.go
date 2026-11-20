// Package postgres stores the audit trail and the approvals.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/ports"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

type txStore struct{ tx pgx.Tx }

func (s *Store) InTx(ctx context.Context, fn func(ports.Tx) error) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error { return fn(&txStore{tx: tx}) })
}

const eventCols = `seq, actor_id, action, target_type, target_id, details, at, prev_hash, hash`

func scanEvent(row pgx.Row) (domain.Event, error) {
	var (
		e   domain.Event
		raw []byte
	)
	if err := row.Scan(&e.Seq, &e.ActorID, &e.Action, &e.TargetType, &e.TargetID, &raw, &e.At, &e.PrevHash, &e.Hash); err != nil {
		return e, err
	}
	return e, json.Unmarshal(raw, &e.Details)
}

// auditLock serializes appends so the chain has no forks.
const auditLock = 0x6175646974 // "audit"

func (t *txStore) AppendAudit(ctx context.Context, e domain.Event, now time.Time) (domain.Event, error) {
	if _, err := t.tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(auditLock)); err != nil {
		return e, err
	}
	last, err := scanEvent(t.tx.QueryRow(ctx, `SELECT `+eventCols+` FROM audit_events ORDER BY seq DESC LIMIT 1`))
	var prev *domain.Event
	switch {
	case err == nil:
		prev = &last
	case !errors.Is(err, pgx.ErrNoRows):
		return e, err
	}
	if e.Details == nil {
		e.Details = map[string]string{}
	}
	e.Chain(prev, now)
	details, err := json.Marshal(e.Details)
	if err != nil {
		return e, err
	}
	_, err = t.tx.Exec(ctx, `INSERT INTO audit_events (`+eventCols+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		e.Seq, e.ActorID, e.Action, e.TargetType, e.TargetID, details, e.At, e.PrevHash, e.Hash)
	return e, err
}

func (s *Store) ListAudit(ctx context.Context, f ports.AuditFilter) ([]domain.Event, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT `+eventCols+` FROM audit_events
		WHERE ($1 = '' OR target_id = $1) AND ($2 = '' OR actor_id = $2) ORDER BY seq DESC LIMIT $3`, f.TargetID, f.ActorID, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ScanAudit(ctx context.Context, visit func(domain.Event) error) error {
	rows, err := s.pool.Query(ctx, `SELECT `+eventCols+` FROM audit_events ORDER BY seq`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return err
		}
		if err := visit(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

const approvalCols = `id, kind, target_id, reason, requested_by, status, decided_by, decision_reason, failure_reason,
	created_at, decided_at, expires_at, params`

func scanApproval(row pgx.Row) (domain.Approval, error) {
	var (
		a            domain.Approval
		kind, status string
	)
	err := row.Scan(&a.ID, &kind, &a.TargetID, &a.Reason, &a.RequestedBy, &status, &a.DecidedBy, &a.DecisionReason,
		&a.FailureReason, &a.CreatedAt, &a.DecidedAt, &a.ExpiresAt, &a.Params)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fmt.Errorf("%w: approval", domain.ErrNotFound)
	}
	a.Kind, a.Status = domain.Kind(kind), domain.Status(status)
	return a, err
}

func collect(rows pgx.Rows, err error) ([]domain.Approval, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Approval
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (t *txStore) InsertApproval(ctx context.Context, a domain.Approval) error {
	_, err := t.tx.Exec(ctx, `INSERT INTO approvals (`+approvalCols+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		a.ID, a.Kind, a.TargetID, a.Reason, a.RequestedBy, a.Status, a.DecidedBy, a.DecisionReason, a.FailureReason,
		a.CreatedAt, a.DecidedAt, a.ExpiresAt, a.Params)
	if pg.IsUniqueViolation(err, "approvals_one_open") {
		return domain.Refuse("APPROVAL_ALREADY_PENDING", "this action is already waiting for a second analyst")
	}
	return err
}

func (t *txStore) ApprovalForUpdate(ctx context.Context, id uuid.UUID) (domain.Approval, error) {
	return scanApproval(t.tx.QueryRow(ctx, `SELECT `+approvalCols+` FROM approvals WHERE id = $1 FOR UPDATE`, id))
}

func (t *txStore) SaveApproval(ctx context.Context, a domain.Approval) error {
	_, err := t.tx.Exec(ctx, `UPDATE approvals SET status = $2, decided_by = $3, decision_reason = $4, failure_reason = $5,
		decided_at = $6, updated_at = now() WHERE id = $1`, a.ID, a.Status, a.DecidedBy, a.DecisionReason, a.FailureReason, a.DecidedAt)
	return err
}

func (t *txStore) ExpirePending(ctx context.Context, now time.Time) ([]domain.Approval, error) {
	return collect(t.tx.Query(ctx, `UPDATE approvals SET status = 'EXPIRED', updated_at = now()
		WHERE status = 'PENDING' AND expires_at <= $1 RETURNING `+approvalCols, now))
}

func (s *Store) Approval(ctx context.Context, id uuid.UUID) (domain.Approval, error) {
	return scanApproval(s.pool.QueryRow(ctx, `SELECT `+approvalCols+` FROM approvals WHERE id = $1`, id))
}

func (s *Store) ListApprovals(ctx context.Context, status domain.Status, limit int) ([]domain.Approval, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return collect(s.pool.Query(ctx, `SELECT `+approvalCols+` FROM approvals WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC LIMIT $2`, string(status), limit))
}

func (s *Store) ApprovedBefore(ctx context.Context, before time.Time) ([]domain.Approval, error) {
	return collect(s.pool.Query(ctx, `SELECT `+approvalCols+` FROM approvals WHERE status = 'APPROVED' AND decided_at < $1
		ORDER BY decided_at LIMIT 100`, before))
}
