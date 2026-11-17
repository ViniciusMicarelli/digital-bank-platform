// Package postgres implements the Pix store.
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

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/inbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/ports"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

const keyCols = `id, account_id, type, value, created_at`

func scanKey(row pgx.Row) (domain.Key, error) {
	var (
		k   domain.Key
		typ string
	)
	err := row.Scan(&k.ID, &k.AccountID, &typ, &k.Value, &k.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return k, fmt.Errorf("%w: pix key", domain.ErrNotFound)
	}
	k.Type = domain.KeyType(typ)
	return k, err
}

func (s *Store) InsertKey(ctx context.Context, k domain.Key, ev outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialize registrations per account so the 5-key limit holds under concurrency.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::TEXT, 0))`, k.AccountID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pix_keys WHERE account_id = $1`, k.AccountID).Scan(&n); err != nil {
			return err
		}
		if n >= domain.MaxKeysPerAccount {
			return domain.ErrTooManyKeys
		}
		_, err := tx.Exec(ctx, `INSERT INTO pix_keys (id, account_id, type, value) VALUES ($1, $2, $3, $4)`,
			k.ID, k.AccountID, k.Type, k.Value)
		if pg.IsUniqueViolation(err, "pix_keys_value_key") {
			return domain.ErrKeyTaken
		}
		if err != nil {
			return err
		}
		return outbox.Insert(ctx, tx, ev)
	})
}

func (s *Store) DeleteKey(ctx context.Context, account, keyID uuid.UUID, event func(domain.Key) (outbox.Message, error)) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		k, err := scanKey(tx.QueryRow(ctx, `DELETE FROM pix_keys WHERE id = $1 AND account_id = $2 RETURNING `+keyCols, keyID, account))
		if err != nil {
			return err
		}
		ev, err := event(k)
		if err != nil {
			return err
		}
		return outbox.Insert(ctx, tx, ev)
	})
}

func (s *Store) Keys(ctx context.Context, account uuid.UUID) ([]domain.Key, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+keyCols+` FROM pix_keys WHERE account_id = $1 ORDER BY created_at`, account)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Key, error) { return scanKey(r) })
}

func (s *Store) KeyByValue(ctx context.Context, value string) (domain.Key, error) {
	return scanKey(s.pool.QueryRow(ctx, `SELECT `+keyCols+` FROM pix_keys WHERE value = $1`, value))
}

const transferCols = `id, idempotency_key, end_to_end_id, direction, status, account_id, key,
	counterparty_name, counterparty_doc, counterparty_ispb, counterparty_bank, internal_account_id,
	amount, currency, description, transaction_id, failure_reason, created_at, updated_at`

func scanTransfer(row pgx.Row) (domain.Transfer, error) {
	var (
		t                       domain.Transfer
		dir, status, currency   string
		amount                  int64
		internal, transactionID *uuid.UUID
	)
	err := row.Scan(&t.ID, &t.IdempotencyKey, &t.EndToEndID, &dir, &status, &t.AccountID, &t.Key,
		&t.Counterparty.Name, &t.Counterparty.DocumentMasked, &t.Counterparty.ISPB, &t.Counterparty.BankName, &internal,
		&amount, &currency, &t.Description, &transactionID, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, fmt.Errorf("%w: pix transfer", domain.ErrNotFound)
	}
	if err != nil {
		return t, err
	}
	t.Direction, t.Status = domain.Direction(dir), domain.Status(status)
	if internal != nil {
		t.InternalAccountID = *internal
	}
	if transactionID != nil {
		t.TransactionID = *transactionID
	}
	t.Amount, err = money.New(amount, money.Currency(currency))
	return t, err
}

func nullable(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func (s *Store) CreateTransfer(ctx context.Context, t domain.Transfer) (*domain.Transfer, []byte, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO pix_transfers (id, idempotency_key, fingerprint, end_to_end_id, direction, status, account_id, key,
		    counterparty_name, counterparty_doc, counterparty_ispb, counterparty_bank, internal_account_id,
		    amount, currency, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		t.ID, t.IdempotencyKey, t.Fingerprint(), t.EndToEndID, t.Direction, t.Status, t.AccountID, t.Key,
		t.Counterparty.Name, t.Counterparty.DocumentMasked, t.Counterparty.ISPB, t.Counterparty.BankName,
		nullable(t.InternalAccountID), t.Amount.Minor(), string(t.Amount.Currency()), t.Description)
	if err != nil || tag.RowsAffected() == 1 {
		return nil, nil, err
	}
	var fp []byte
	if err := s.pool.QueryRow(ctx, `SELECT fingerprint FROM pix_transfers WHERE idempotency_key = $1`, t.IdempotencyKey).Scan(&fp); err != nil {
		return nil, nil, err
	}
	existing, err := scanTransfer(s.pool.QueryRow(ctx, `SELECT `+transferCols+` FROM pix_transfers WHERE idempotency_key = $1`, t.IdempotencyKey))
	return &existing, fp, err
}

func (s *Store) Transfer(ctx context.Context, id uuid.UUID) (domain.Transfer, error) {
	return scanTransfer(s.pool.QueryRow(ctx, `SELECT `+transferCols+` FROM pix_transfers WHERE id = $1`, id))
}

func (s *Store) Acquire(ctx context.Context, id uuid.UUID, d time.Duration) (domain.Transfer, bool, error) {
	t, err := scanTransfer(s.pool.QueryRow(ctx, `
		UPDATE pix_transfers SET locked_until = now() + $2::INTERVAL
		 WHERE id = $1 AND (locked_until IS NULL OR locked_until < now())
		RETURNING `+transferCols, id, d.String()))
	if errors.Is(err, domain.ErrNotFound) {
		current, err := s.Transfer(ctx, id)
		return current, false, err
	}
	return t, err == nil, err
}

func (s *Store) Release(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE pix_transfers SET locked_until = NULL WHERE id = $1`, id)
	return err
}

func (s *Store) SaveTransfer(ctx context.Context, t domain.Transfer, events ...outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE pix_transfers SET status = $2, transaction_id = $3, failure_reason = $4, updated_at = now()
		                         WHERE id = $1`, t.ID, t.Status, nullable(t.TransactionID), t.FailureReason)
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

func (s *Store) Unfinished(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM pix_transfers
		 WHERE status IN ('CREATED', 'FUNDS_RESERVED', 'SENT_TO_SPI') AND updated_at < $1
		   AND (locked_until IS NULL OR locked_until < now())
		 ORDER BY updated_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// HandleAccountEvent removes the Pix keys of closed accounts. It runs inside
// the inbox transaction, so a redelivered event is a no-op.
func (s *Store) HandleAccountEvent(ctx context.Context, ev outbox.Event) error {
	if ev.Type != "account.closed" {
		return nil
	}
	var p struct {
		AccountID string `json:"account_id"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return fmt.Errorf("decode %s: %w", ev.Type, err)
	}
	return inbox.Handle(ctx, s.pool, "pix-service.account-closed", ev, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM pix_keys WHERE account_id = $1`, p.AccountID)
		return err
	})
}
