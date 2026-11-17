// Package postgres implements the ledger ports on PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/ports"
)

// Store implements ports.Store.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

func (s *Store) InTx(ctx context.Context, fn func(ports.Tx) error) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error { return fn(&txStore{tx: tx}) })
}

// querier is satisfied by both the pool and a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const accountCols = `id, kind, currency, overdraft_limit, COALESCE(code, ''), created_at`

func scanAccount(row pgx.Row) (domain.Account, error) {
	var a domain.Account
	var kind, currency string
	if err := row.Scan(&a.ID, &kind, &currency, &a.OverdraftLimit, &a.Code, &a.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Account{}, domain.ErrNotFound
		}
		return domain.Account{}, err
	}
	a.Kind, a.Currency = domain.AccountKind(kind), money.Currency(currency)
	return a, nil
}

func (s *Store) Account(ctx context.Context, id uuid.UUID) (domain.Account, error) {
	a, err := scanAccount(s.pool.QueryRow(ctx, `SELECT `+accountCols+` FROM ledger_accounts WHERE id = $1`, id))
	if errors.Is(err, domain.ErrNotFound) {
		return a, fmt.Errorf("%w: account %s", domain.ErrNotFound, id)
	}
	return a, err
}

func (s *Store) Entry(ctx context.Context, id uuid.UUID) (domain.Entry, error) {
	return loadEntry(ctx, s.pool, `WHERE e.id = $1`, id)
}

func (s *Store) Balance(ctx context.Context, accountID uuid.UUID) (domain.Balance, error) {
	b := domain.Balance{AccountID: accountID}
	err := s.pool.QueryRow(ctx, `SELECT posted, pending FROM balances WHERE account_id = $1`, accountID).
		Scan(&b.Posted, &b.Pending)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, fmt.Errorf("%w: account %s", domain.ErrNotFound, accountID)
	}
	return b, err
}

func (s *Store) PostedAt(ctx context.Context, accountID uuid.UUID, at time.Time) (int64, error) {
	var sum int64
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)::BIGINT FROM postings WHERE account_id = $1 AND created_at <= $2`,
		accountID, at).Scan(&sum)
	return sum, err
}

func (s *Store) Postings(ctx context.Context, accountID uuid.UUID, before int64, limit int) ([]ports.AccountPosting, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.entry_id, p.amount, p.currency, e.description, p.created_at
		  FROM postings p JOIN journal_entries e ON e.id = p.entry_id
		 WHERE p.account_id = $1 AND ($2 = 0 OR p.id < $2)
		 ORDER BY p.id DESC
		 LIMIT $3`, accountID, before, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ports.AccountPosting, error) {
		var p ports.AccountPosting
		err := r.Scan(&p.ID, &p.EntryID, &p.Amount, &p.Currency, &p.Description, &p.CreatedAt)
		return p, err
	})
}

// loadEntry reads an entry and its postings. where filters journal_entries e.
func loadEntry(ctx context.Context, q querier, where string, args ...any) (domain.Entry, error) {
	var (
		e       domain.Entry
		reverse *uuid.UUID
	)
	err := q.QueryRow(ctx, `
		SELECT e.id, e.idempotency_key, e.description, e.metadata, e.reverses_entry_id, e.created_at
		  FROM journal_entries e `+where, args...).
		Scan(&e.ID, &e.IdempotencyKey, &e.Description, &e.Metadata, &reverse, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Entry{}, fmt.Errorf("%w: entry", domain.ErrNotFound)
	}
	if err != nil {
		return domain.Entry{}, err
	}
	if reverse != nil {
		e.ReversesEntryID = *reverse
	}
	rows, err := q.Query(ctx, `SELECT account_id, amount, currency FROM postings WHERE entry_id = $1 ORDER BY id`, e.ID)
	if err != nil {
		return domain.Entry{}, err
	}
	e.Postings, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Posting, error) {
		var (
			id       uuid.UUID
			amount   int64
			currency string
		)
		if err := r.Scan(&id, &amount, &currency); err != nil {
			return domain.Posting{}, err
		}
		m, err := money.New(amount, money.Currency(currency))
		return domain.Posting{AccountID: id, Amount: m}, err
	})
	return e, err
}

// txStore implements ports.Tx.
type txStore struct {
	tx pgx.Tx
}

func (t *txStore) InsertAccount(ctx context.Context, a domain.Account) (domain.Account, bool, error) {
	var code *string
	if a.Code != "" {
		code = &a.Code
	}
	tag, err := t.tx.Exec(ctx, `
		INSERT INTO ledger_accounts (id, kind, currency, overdraft_limit, code)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO NOTHING`,
		a.ID, a.Kind, a.Currency, a.OverdraftLimit, code)
	if err != nil {
		if pg.IsUniqueViolation(err, "ledger_accounts_code_key") {
			return domain.Account{}, false, fmt.Errorf("%w: code %q is taken", domain.ErrAccountConflict, a.Code)
		}
		return domain.Account{}, false, err
	}
	created := tag.RowsAffected() == 1
	if created {
		if _, err := t.tx.Exec(ctx, `INSERT INTO balances (account_id) VALUES ($1)`, a.ID); err != nil {
			return domain.Account{}, false, err
		}
	}
	stored, err := scanAccount(t.tx.QueryRow(ctx, `SELECT `+accountCols+` FROM ledger_accounts WHERE id = $1`, a.ID))
	return stored, created, err
}

func (t *txStore) Accounts(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Account, error) {
	rows, err := t.tx.Query(ctx, `SELECT `+accountCols+` FROM ledger_accounts WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	accounts := make(map[uuid.UUID]domain.Account, len(ids))
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts[a.ID] = a
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, ok := accounts[id]; !ok {
			return nil, fmt.Errorf("%w: account %s", domain.ErrNotFound, id)
		}
	}
	return accounts, nil
}

func (t *txStore) LockBalances(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*domain.Balance, error) {
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	sorted = slices.Compact(sorted)

	// ORDER BY makes Postgres take the row locks in a deterministic order.
	rows, err := t.tx.Query(ctx, `
		SELECT account_id, posted, pending FROM balances
		 WHERE account_id = ANY($1) ORDER BY account_id FOR UPDATE`, sorted)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]*domain.Balance, len(sorted))
	for rows.Next() {
		b := &domain.Balance{}
		if err := rows.Scan(&b.AccountID, &b.Posted, &b.Pending); err != nil {
			return nil, err
		}
		out[b.AccountID] = b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(sorted) {
		return nil, fmt.Errorf("%w: balance row missing", domain.ErrNotFound)
	}
	return out, nil
}

func (t *txStore) SaveBalances(ctx context.Context, balances map[uuid.UUID]*domain.Balance) error {
	batch := &pgx.Batch{}
	for _, b := range balances {
		batch.Queue(`UPDATE balances SET posted = $2, pending = $3, version = version + 1, updated_at = now()
		              WHERE account_id = $1`, b.AccountID, b.Posted, b.Pending)
	}
	return t.tx.SendBatch(ctx, batch).Close()
}

func (t *txStore) ClaimEntry(ctx context.Context, e domain.Entry) (*domain.Entry, []byte, error) {
	var reverses *uuid.UUID
	if e.ReversesEntryID != uuid.Nil {
		reverses = &e.ReversesEntryID
	}
	tag, err := t.tx.Exec(ctx, `
		INSERT INTO journal_entries (id, idempotency_key, fingerprint, description, metadata, reverses_entry_id)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (idempotency_key) DO NOTHING`,
		e.ID, e.IdempotencyKey, e.Fingerprint(), e.Description, e.Metadata, reverses)
	if err != nil {
		if pg.IsUniqueViolation(err, "journal_entries_reverses_entry_id_key") {
			return nil, nil, fmt.Errorf("%w: %s", domain.ErrAlreadyReversed, e.ReversesEntryID)
		}
		return nil, nil, err
	}
	if tag.RowsAffected() == 1 {
		return nil, nil, nil
	}
	var fp []byte
	if err := t.tx.QueryRow(ctx, `SELECT fingerprint FROM journal_entries WHERE idempotency_key = $1`,
		e.IdempotencyKey).Scan(&fp); err != nil {
		return nil, nil, err
	}
	existing, err := loadEntry(ctx, t.tx, `WHERE e.idempotency_key = $1`, e.IdempotencyKey)
	if err != nil {
		return nil, nil, err
	}
	return &existing, fp, nil
}

func (t *txStore) InsertPostings(ctx context.Context, e domain.Entry) error {
	rows := make([][]any, len(e.Postings))
	for i, p := range e.Postings {
		rows[i] = []any{e.ID, p.AccountID, p.Amount.Minor(), string(p.Amount.Currency())}
	}
	_, err := t.tx.CopyFrom(ctx, pgx.Identifier{"postings"},
		[]string{"entry_id", "account_id", "amount", "currency"}, pgx.CopyFromRows(rows))
	return err
}

func (t *txStore) EntryForUpdate(ctx context.Context, id uuid.UUID) (domain.Entry, error) {
	e, err := loadEntry(ctx, t.tx, `WHERE e.id = $1 FOR UPDATE`, id)
	if errors.Is(err, domain.ErrNotFound) {
		return e, fmt.Errorf("%w: entry %s", domain.ErrNotFound, id)
	}
	return e, err
}

func (t *txStore) ReversalOf(ctx context.Context, id uuid.UUID) (*domain.Entry, error) {
	e, err := loadEntry(ctx, t.tx, `WHERE e.reverses_entry_id = $1`, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

const holdCols = `id, idempotency_key, account_id, counter_account_id, amount, currency, status,
	capture_entry_id, description, created_at`

func scanHold(row pgx.Row) (domain.Hold, error) {
	var (
		h        domain.Hold
		amount   int64
		currency string
		status   string
		capture  *uuid.UUID
	)
	err := row.Scan(&h.ID, &h.IdempotencyKey, &h.AccountID, &h.CounterAccountID, &amount, &currency,
		&status, &capture, &h.Description, &h.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Hold{}, fmt.Errorf("%w: hold", domain.ErrNotFound)
	}
	if err != nil {
		return domain.Hold{}, err
	}
	if h.Amount, err = money.New(amount, money.Currency(currency)); err != nil {
		return domain.Hold{}, err
	}
	h.Status = domain.HoldStatus(status)
	if capture != nil {
		h.CaptureEntryID = *capture
	}
	return h, nil
}

func (t *txStore) ClaimHold(ctx context.Context, h domain.Hold) (*domain.Hold, []byte, error) {
	tag, err := t.tx.Exec(ctx, `
		INSERT INTO holds (id, idempotency_key, fingerprint, account_id, counter_account_id, amount, currency, status, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (idempotency_key) DO NOTHING`,
		h.ID, h.IdempotencyKey, h.Fingerprint(), h.AccountID, h.CounterAccountID,
		h.Amount.Minor(), string(h.Amount.Currency()), h.Status, h.Description)
	if err != nil {
		return nil, nil, err
	}
	if tag.RowsAffected() == 1 {
		return nil, nil, nil
	}
	var fp []byte
	if err := t.tx.QueryRow(ctx, `SELECT fingerprint FROM holds WHERE idempotency_key = $1`,
		h.IdempotencyKey).Scan(&fp); err != nil {
		return nil, nil, err
	}
	existing, err := scanHold(t.tx.QueryRow(ctx, `SELECT `+holdCols+` FROM holds WHERE idempotency_key = $1`, h.IdempotencyKey))
	if err != nil {
		return nil, nil, err
	}
	return &existing, fp, nil
}

func (t *txStore) HoldForUpdate(ctx context.Context, id uuid.UUID) (domain.Hold, error) {
	h, err := scanHold(t.tx.QueryRow(ctx, `SELECT `+holdCols+` FROM holds WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, domain.ErrNotFound) {
		return h, fmt.Errorf("%w: hold %s", domain.ErrNotFound, id)
	}
	return h, err
}

func (t *txStore) UpdateHold(ctx context.Context, h domain.Hold) error {
	var capture *uuid.UUID
	if h.CaptureEntryID != uuid.Nil {
		capture = &h.CaptureEntryID
	}
	_, err := t.tx.Exec(ctx, `UPDATE holds SET status = $2, capture_entry_id = $3, updated_at = now() WHERE id = $1`,
		h.ID, h.Status, capture)
	return err
}

func (t *txStore) Publish(ctx context.Context, msg outbox.Message) error {
	return outbox.Insert(ctx, t.tx, msg)
}
