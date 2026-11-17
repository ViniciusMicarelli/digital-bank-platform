// Package postgres stores boleto payments.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/ports"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

const cols = `id, idempotency_key, account_id, status, failure_reason, barcode, line, payee_name, payee_document, payee_bank,
	due_date, nominal, fine, interest, scheduled_for, transaction_id, created_at, updated_at`

func scan(row pgx.Row) (domain.Payment, error) {
	var (
		p      domain.Payment
		status string
		due    *time.Time
		txID   *uuid.UUID
	)
	err := row.Scan(&p.ID, &p.IdempotencyKey, &p.AccountID, &status, &p.FailureReason, &p.Barcode, &p.Line, &p.Payee.Name,
		&p.Payee.DocumentMasked, &p.Payee.BankName, &due, &p.Nominal, &p.Fine, &p.Interest, &p.ScheduledFor, &txID,
		&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, fmt.Errorf("%w: payment", domain.ErrNotFound)
	}
	p.Status = domain.Status(status)
	if due != nil {
		p.DueDate = *due
	}
	if txID != nil {
		p.TransactionID = *txID
	}
	return p, err
}

func nullDate(d time.Time) *time.Time {
	if d.IsZero() {
		return nil
	}
	return &d
}

func nullID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func (s *Store) Insert(ctx context.Context, p domain.Payment, fingerprint string) (*domain.Payment, string, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO boleto_payments (id, idempotency_key, fingerprint, account_id, status, failure_reason,
		barcode, line, payee_name, payee_document, payee_bank, due_date, nominal, fine, interest, scheduled_for, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		p.ID, p.IdempotencyKey, fingerprint, p.AccountID, p.Status, p.FailureReason, p.Barcode, p.Line, p.Payee.Name,
		p.Payee.DocumentMasked, p.Payee.BankName, nullDate(p.DueDate), p.Nominal, p.Fine, p.Interest, p.ScheduledFor, p.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "boleto_payments_live" {
		return nil, "", domain.Refuse("DUPLICATE_PAYMENT", "this boleto is already paid or being paid")
	}
	if err != nil || tag.RowsAffected() == 1 {
		return nil, "", err
	}
	var fp string
	if err := s.pool.QueryRow(ctx, `SELECT fingerprint FROM boleto_payments WHERE idempotency_key = $1`, p.IdempotencyKey).Scan(&fp); err != nil {
		return nil, "", err
	}
	existing, err := scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM boleto_payments WHERE idempotency_key = $1`, p.IdempotencyKey))
	return &existing, fp, err
}

func (s *Store) Payment(ctx context.Context, id uuid.UUID) (domain.Payment, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM boleto_payments WHERE id = $1`, id))
}

func (s *Store) Payments(ctx context.Context, account uuid.UUID, limit int) ([]domain.Payment, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+cols+` FROM boleto_payments WHERE account_id = $1 ORDER BY created_at DESC LIMIT $2`, account, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Payment, error) { return scan(r) })
}

func (s *Store) Acquire(ctx context.Context, id uuid.UUID, lease time.Duration) (domain.Payment, bool, error) {
	p, err := scan(s.pool.QueryRow(ctx, `UPDATE boleto_payments SET lease_until = now() + $2::interval
		WHERE id = $1 AND (lease_until IS NULL OR lease_until < now()) RETURNING `+cols, id, lease.String()))
	if errors.Is(err, domain.ErrNotFound) {
		p, err = s.Payment(ctx, id)
		return p, false, err
	}
	return p, err == nil, err
}

func (s *Store) Release(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE boleto_payments SET lease_until = NULL WHERE id = $1`, id)
	return err
}

func (s *Store) Save(ctx context.Context, p domain.Payment, events ...outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE boleto_payments SET status = $2, failure_reason = $3, payee_name = $4, payee_document = $5,
			payee_bank = $6, due_date = $7, nominal = $8, fine = $9, interest = $10, transaction_id = $11, updated_at = now()
			WHERE id = $1`, p.ID, p.Status, p.FailureReason, p.Payee.Name, p.Payee.DocumentMasked, p.Payee.BankName,
			nullDate(p.DueDate), p.Nominal, p.Fine, p.Interest, nullID(p.TransactionID))
		if err != nil {
			return err
		}
		for _, ev := range events {
			if err := outbox.Insert(ctx, tx, ev); err != nil {
				return err
			}
		}
		return nil
	})
}

func ids(rows pgx.Rows, err error) ([]uuid.UUID, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (s *Store) Unfinished(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error) {
	return ids(s.pool.Query(ctx, `SELECT id FROM boleto_payments
		WHERE status IN ('CREATED', 'FUNDS_RESERVED', 'SENT_TO_CLEARING') AND updated_at < $1 ORDER BY updated_at LIMIT $2`, before, limit))
}

func (s *Store) Due(ctx context.Context, today time.Time, limit int) ([]uuid.UUID, error) {
	return ids(s.pool.Query(ctx, `SELECT id FROM boleto_payments WHERE status = 'SCHEDULED' AND scheduled_for <= $1
		ORDER BY scheduled_for LIMIT $2`, today, limit))
}

func (s *Store) InsertSandboxBoleto(ctx context.Context, barcode, payeeName, payeeDocument string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sandbox_boletos (barcode, payee_name, payee_document) VALUES ($1, $2, $3)
		ON CONFLICT (barcode) DO NOTHING`, barcode, payeeName, payeeDocument)
	return err
}

// SandboxPayee returns the payee of a sandbox boleto, if it is one.
func (s *Store) SandboxPayee(ctx context.Context, barcode string) (name, document string, ok bool, err error) {
	err = s.pool.QueryRow(ctx, `SELECT payee_name, payee_document FROM sandbox_boletos WHERE barcode = $1`, barcode).Scan(&name, &document)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	return name, document, err == nil, err
}
