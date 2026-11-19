// Package postgres stores credit lines, holds, invoices and payments.
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
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/ports"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", domain.ErrNotFound, what)
	}
	return err
}

// ---- scans ----

const lineCols = `id, account_id, idempotency_key, status, credit_limit, used, closing_day, interest_rate_bp, created_at`

func scanLine(row pgx.Row) (domain.Line, error) {
	var (
		l      domain.Line
		status string
	)
	err := row.Scan(&l.ID, &l.AccountID, &l.IdempotencyKey, &status, &l.Limit, &l.Used, &l.ClosingDay, &l.InterestRateBP, &l.CreatedAt)
	l.Status = domain.LineStatus(status)
	return l, notFound(err, "credit line")
}

const invoiceCols = `id, line_id, status, closing_date, due_date, total, paid, pending`

func scanInvoice(row pgx.Row) (domain.Invoice, error) {
	var (
		i      domain.Invoice
		status string
	)
	err := row.Scan(&i.ID, &i.LineID, &status, &i.ClosingDate, &i.DueDate, &i.Total, &i.Paid, &i.Pending)
	i.Status = domain.InvoiceStatus(status)
	return i, notFound(err, "invoice")
}

func collectInvoices(rows pgx.Rows, err error) ([]domain.Invoice, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Invoice, error) { return scanInvoice(r) })
}

const holdCols = `id, idempotency_key, line_id, account_id, status, failure_reason, amount, captured, description, metadata, created_at, installments`

func scanHold(row pgx.Row) (domain.Hold, error) {
	var (
		h      domain.Hold
		line   *uuid.UUID
		status string
	)
	err := row.Scan(&h.ID, &h.IdempotencyKey, &line, &h.AccountID, &status, &h.FailureReason, &h.Amount, &h.Captured,
		&h.Description, &h.Metadata, &h.CreatedAt, &h.Installments)
	if line != nil {
		h.LineID = *line
	}
	h.Status = domain.HoldStatus(status)
	return h, notFound(err, "hold")
}

const paymentCols = `id, idempotency_key, invoice_id, account_id, status, failure_reason, amount, created_at`

func scanPayment(row pgx.Row) (domain.Payment, error) {
	var (
		p      domain.Payment
		status string
	)
	err := row.Scan(&p.ID, &p.IdempotencyKey, &p.InvoiceID, &p.AccountID, &status, &p.FailureReason, &p.Amount, &p.CreatedAt)
	p.Status = domain.PaymentStatus(status)
	return p, notFound(err, "payment")
}

// ---- reads ----

func (s *Store) InTx(ctx context.Context, fn func(ports.Tx) error) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error { return fn(&unit{ctx: ctx, tx: tx}) })
}

func (s *Store) Line(ctx context.Context, id uuid.UUID) (domain.Line, error) {
	return scanLine(s.pool.QueryRow(ctx, `SELECT `+lineCols+` FROM credit_lines WHERE id = $1`, id))
}

func (s *Store) LineByAccount(ctx context.Context, account uuid.UUID) (domain.Line, error) {
	return scanLine(s.pool.QueryRow(ctx, `SELECT `+lineCols+` FROM credit_lines WHERE account_id = $1`, account))
}

func (s *Store) Invoices(ctx context.Context, line uuid.UUID) ([]domain.Invoice, error) {
	return collectInvoices(s.pool.Query(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE line_id = $1 ORDER BY closing_date DESC`, line))
}

func (s *Store) Invoice(ctx context.Context, id uuid.UUID) (domain.Invoice, error) {
	return scanInvoice(s.pool.QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE id = $1`, id))
}

func (s *Store) Items(ctx context.Context, invoice uuid.UUID) ([]domain.Item, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, invoice_id, kind, amount, description, reference, occurred_at
		FROM invoice_items WHERE invoice_id = $1 ORDER BY occurred_at, id`, invoice)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Item, error) {
		var (
			it   domain.Item
			kind string
		)
		err := r.Scan(&it.ID, &it.InvoiceID, &kind, &it.Amount, &it.Description, &it.Reference, &it.OccurredAt)
		it.Kind = domain.ItemKind(kind)
		return it, err
	})
}

func (s *Store) RefundByKey(ctx context.Context, key string) (*domain.Refund, error) {
	var r domain.Refund
	err := s.pool.QueryRow(ctx, `SELECT id, idempotency_key, line_id, account_id, invoice_id, amount, description, created_at
		FROM refunds WHERE idempotency_key = $1`, key).
		Scan(&r.ID, &r.IdempotencyKey, &r.LineID, &r.AccountID, &r.InvoiceID, &r.Amount, &r.Description, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &r, err
}

func (u *unit) InsertRefund(r domain.Refund) (bool, error) {
	tag, err := u.tx.Exec(u.ctx, `INSERT INTO refunds (id, idempotency_key, line_id, account_id, invoice_id, amount, description, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (idempotency_key) DO NOTHING`,
		r.ID, r.IdempotencyKey, r.LineID, r.AccountID, r.InvoiceID, r.Amount, r.Description, r.CreatedAt)
	return tag.RowsAffected() == 1, err
}

func (s *Store) Hold(ctx context.Context, id uuid.UUID) (domain.Hold, error) {
	return scanHold(s.pool.QueryRow(ctx, `SELECT `+holdCols+` FROM holds WHERE id = $1`, id))
}

func ids(rows pgx.Rows, err error) ([]uuid.UUID, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (s *Store) LinesDue(ctx context.Context, today time.Time, limit int) ([]uuid.UUID, error) {
	return ids(s.pool.Query(ctx, `
		SELECT DISTINCT line_id FROM (
			(SELECT line_id FROM invoices WHERE status = 'OPEN' AND closing_date <= $1 LIMIT $2)
			UNION ALL
			(SELECT line_id FROM invoices WHERE status = 'CLOSED' AND due_date < $1 AND pending = 0 AND paid < total LIMIT $2)
		) due`, today, limit))
}

func (s *Store) UnpostedCharges(ctx context.Context, limit int) ([]domain.Invoice, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+invoiceCols+`, charges FROM invoices WHERE NOT charges_posted LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Invoice, error) {
		var (
			i      domain.Invoice
			status string
		)
		err := r.Scan(&i.ID, &i.LineID, &status, &i.ClosingDate, &i.DueDate, &i.Total, &i.Paid, &i.Pending, &i.Charges)
		i.Status = domain.InvoiceStatus(status)
		return i, err
	})
}

func (s *Store) MarkChargesPosted(ctx context.Context, invoice uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE invoices SET charges_posted = true, updated_at = now() WHERE id = $1`, invoice)
	return err
}

func (s *Store) ExpiredHolds(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error) {
	return ids(s.pool.Query(ctx, `SELECT id FROM holds WHERE status = 'RESERVED' AND created_at < $1 ORDER BY created_at LIMIT $2`, before, limit))
}

func (s *Store) PendingPayments(ctx context.Context, before time.Time, limit int) ([]domain.Payment, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+paymentCols+` FROM payments WHERE status = 'PENDING' AND created_at < $1 ORDER BY created_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Payment, error) { return scanPayment(r) })
}

// ---- unit of work ----

type unit struct {
	ctx context.Context
	tx  pgx.Tx
}

func (u *unit) exec(sql string, args ...any) error {
	_, err := u.tx.Exec(u.ctx, sql, args...)
	return err
}

func (u *unit) InsertLine(l domain.Line) (bool, error) {
	tag, err := u.tx.Exec(u.ctx, `INSERT INTO credit_lines (id, account_id, idempotency_key, status, credit_limit, used,
		closing_day, interest_rate_bp) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT DO NOTHING`,
		l.ID, l.AccountID, l.IdempotencyKey, l.Status, l.Limit, l.Used, l.ClosingDay, l.InterestRateBP)
	return tag.RowsAffected() == 1, err
}

func (u *unit) LockLine(id uuid.UUID) (domain.Line, error) {
	return scanLine(u.tx.QueryRow(u.ctx, `SELECT `+lineCols+` FROM credit_lines WHERE id = $1 FOR UPDATE`, id))
}

func (u *unit) LockLineByAccount(account uuid.UUID) (domain.Line, error) {
	return scanLine(u.tx.QueryRow(u.ctx, `SELECT `+lineCols+` FROM credit_lines WHERE account_id = $1 FOR UPDATE`, account))
}

func (u *unit) SaveLine(l domain.Line) error {
	return u.exec(`UPDATE credit_lines SET status = $2, credit_limit = $3, used = $4, updated_at = now() WHERE id = $1`,
		l.ID, l.Status, l.Limit, l.Used)
}

func (u *unit) InsertInvoice(i domain.Invoice) error {
	return u.exec(`INSERT INTO invoices (id, line_id, status, closing_date, due_date, total, paid, pending)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, i.ID, i.LineID, i.Status, i.ClosingDate, i.DueDate, i.Total, i.Paid, i.Pending)
}

func (u *unit) LockOpenInvoice(line uuid.UUID) (domain.Invoice, error) {
	return scanInvoice(u.tx.QueryRow(u.ctx, `SELECT `+invoiceCols+` FROM invoices WHERE line_id = $1 AND status = 'OPEN' FOR UPDATE`, line))
}

func (u *unit) LockInvoice(id uuid.UUID) (domain.Invoice, error) {
	return scanInvoice(u.tx.QueryRow(u.ctx, `SELECT `+invoiceCols+` FROM invoices WHERE id = $1 FOR UPDATE`, id))
}

func (u *unit) LockClosedInvoices(line uuid.UUID) ([]domain.Invoice, error) {
	return collectInvoices(u.tx.Query(u.ctx, `SELECT `+invoiceCols+` FROM invoices WHERE line_id = $1 AND status = 'CLOSED'
		ORDER BY closing_date FOR UPDATE`, line))
}

func (u *unit) SaveInvoice(i domain.Invoice) error {
	return u.exec(`UPDATE invoices SET status = $2, closing_date = $3, due_date = $4, total = $5, paid = $6, pending = $7,
		updated_at = now() WHERE id = $1`, i.ID, i.Status, i.ClosingDate, i.DueDate, i.Total, i.Paid, i.Pending)
}

func (u *unit) SetCharges(invoice uuid.UUID, charges int64) error {
	return u.exec(`UPDATE invoices SET charges = $2, charges_posted = $3 WHERE id = $1`, invoice, charges, charges == 0)
}

func (u *unit) AddItem(it domain.Item) error {
	return u.exec(`INSERT INTO invoice_items (id, invoice_id, kind, amount, description, reference, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (invoice_id, kind, reference) DO NOTHING`,
		it.ID, it.InvoiceID, it.Kind, it.Amount, it.Description, it.Reference, it.OccurredAt)
}

func (u *unit) HoldByKey(key string) (*domain.Hold, error) {
	h, err := scanHold(u.tx.QueryRow(u.ctx, `SELECT `+holdCols+` FROM holds WHERE idempotency_key = $1`, key))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return &h, err
}

func (u *unit) LockHold(id uuid.UUID) (domain.Hold, error) {
	return scanHold(u.tx.QueryRow(u.ctx, `SELECT `+holdCols+` FROM holds WHERE id = $1 FOR UPDATE`, id))
}

func nullable(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func (u *unit) InsertHold(h domain.Hold) error {
	if h.Metadata == nil {
		h.Metadata = map[string]string{}
	}
	if h.Installments == 0 {
		h.Installments = 1
	}
	return u.exec(`INSERT INTO holds (id, idempotency_key, line_id, account_id, status, failure_reason, amount, captured,
		description, metadata, created_at, installments) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		h.ID, h.IdempotencyKey, nullable(h.LineID), h.AccountID, h.Status, h.FailureReason, h.Amount, h.Captured, h.Description,
		h.Metadata, h.CreatedAt, h.Installments)
}

func (u *unit) SaveHold(h domain.Hold) error {
	return u.exec(`UPDATE holds SET status = $2, captured = $3, updated_at = now() WHERE id = $1`, h.ID, h.Status, h.Captured)
}

func (u *unit) PaymentByKey(key string) (*domain.Payment, error) {
	p, err := scanPayment(u.tx.QueryRow(u.ctx, `SELECT `+paymentCols+` FROM payments WHERE idempotency_key = $1`, key))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return &p, err
}

func (u *unit) LockPayment(id uuid.UUID) (domain.Payment, error) {
	return scanPayment(u.tx.QueryRow(u.ctx, `SELECT `+paymentCols+` FROM payments WHERE id = $1 FOR UPDATE`, id))
}

func (u *unit) InsertPayment(p domain.Payment) error {
	return u.exec(`INSERT INTO payments (id, idempotency_key, invoice_id, account_id, status, failure_reason, amount, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, p.ID, p.IdempotencyKey, p.InvoiceID, p.AccountID, p.Status, p.FailureReason, p.Amount, p.CreatedAt)
}

func (u *unit) SavePayment(p domain.Payment) error {
	return u.exec(`UPDATE payments SET status = $2, failure_reason = $3, updated_at = now() WHERE id = $1`, p.ID, p.Status, p.FailureReason)
}

func (u *unit) Publish(m outbox.Message) error { return outbox.Insert(u.ctx, u.tx, m) }

// ---- installments ----

func (u *unit) InsertInstallments(list []domain.Installment) error {
	for _, i := range list {
		if err := u.exec(`INSERT INTO installments (id, line_id, hold_id, number, total, amount, description, invoice_id, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (hold_id, number) DO NOTHING`,
			i.ID, i.LineID, i.HoldID, i.Number, i.Total, i.Amount, i.Description, i.InvoiceID, i.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (u *unit) NextInstallments(line uuid.UUID) ([]domain.Installment, error) {
	rows, err := u.tx.Query(u.ctx, `SELECT DISTINCT ON (hold_id) id, line_id, hold_id, number, total, amount, description, created_at
		FROM installments WHERE line_id = $1 AND invoice_id IS NULL ORDER BY hold_id, number`, line)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Installment
	for rows.Next() {
		var i domain.Installment
		if err := rows.Scan(&i.ID, &i.LineID, &i.HoldID, &i.Number, &i.Total, &i.Amount, &i.Description, &i.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (u *unit) BillInstallment(id, invoice uuid.UUID) error {
	return u.exec(`UPDATE installments SET invoice_id = $2 WHERE id = $1 AND invoice_id IS NULL`, id, invoice)
}

func (s *Store) UpcomingInstallments(ctx context.Context, line uuid.UUID) (int64, error) {
	var sum int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(sum(amount), 0) FROM installments WHERE line_id = $1 AND invoice_id IS NULL`, line).Scan(&sum)
	return sum, err
}

func (s *Store) InstallmentPlans(ctx context.Context, line uuid.UUID) ([]domain.InstallmentPlan, error) {
	rows, err := s.pool.Query(ctx, `SELECT i.hold_id, min(i.description), h.captured, max(i.total),
			count(*) FILTER (WHERE i.invoice_id IS NOT NULL), min(i.amount),
			COALESCE(sum(i.amount) FILTER (WHERE i.invoice_id IS NULL), 0), h.created_at
		FROM installments i JOIN holds h ON h.id = i.hold_id
		WHERE i.line_id = $1 GROUP BY i.hold_id, h.captured, h.created_at
		HAVING count(*) FILTER (WHERE i.invoice_id IS NULL) > 0 ORDER BY h.created_at DESC`, line)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.InstallmentPlan
	for rows.Next() {
		var p domain.InstallmentPlan
		if err := rows.Scan(&p.HoldID, &p.Description, &p.Total, &p.Count, &p.Billed, &p.Installment, &p.Remaining, &p.PurchasedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
