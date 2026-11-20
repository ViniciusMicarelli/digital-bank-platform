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
	"github.com/jackc/pgx/v5/pgconn"
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
	amount, currency, description, transaction_id, failure_reason, COALESCE(qr_txid, ''), refund_of,
	refunded_amount, created_at, updated_at`

func scanTransfer(row pgx.Row) (domain.Transfer, error) {
	var (
		t                                 domain.Transfer
		dir, status, currency             string
		amount                            int64
		internal, transactionID, refundOf *uuid.UUID
	)
	err := row.Scan(&t.ID, &t.IdempotencyKey, &t.EndToEndID, &dir, &status, &t.AccountID, &t.Key,
		&t.Counterparty.Name, &t.Counterparty.DocumentMasked, &t.Counterparty.ISPB, &t.Counterparty.BankName, &internal,
		&amount, &currency, &t.Description, &transactionID, &t.FailureReason, &t.QRTxID, &refundOf,
		&t.Refunded, &t.CreatedAt, &t.UpdatedAt)
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
	if refundOf != nil {
		t.RefundOf = *refundOf
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

func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// querier is a pool or a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func insertTransfer(ctx context.Context, q querier, t domain.Transfer) (*domain.Transfer, []byte, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO pix_transfers (id, idempotency_key, fingerprint, end_to_end_id, direction, status, account_id, key,
		    counterparty_name, counterparty_doc, counterparty_ispb, counterparty_bank, internal_account_id,
		    amount, currency, description, qr_txid, refund_of)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		t.ID, t.IdempotencyKey, t.Fingerprint(), t.EndToEndID, t.Direction, t.Status, t.AccountID, t.Key,
		t.Counterparty.Name, t.Counterparty.DocumentMasked, t.Counterparty.ISPB, t.Counterparty.BankName,
		nullable(t.InternalAccountID), t.Amount.Minor(), string(t.Amount.Currency()), t.Description,
		nullableText(t.QRTxID), nullable(t.RefundOf))
	if err != nil || tag.RowsAffected() == 1 {
		return nil, nil, err
	}
	var fp []byte
	if err := q.QueryRow(ctx, `SELECT fingerprint FROM pix_transfers WHERE idempotency_key = $1`, t.IdempotencyKey).Scan(&fp); err != nil {
		return nil, nil, err
	}
	existing, err := scanTransfer(q.QueryRow(ctx, `SELECT `+transferCols+` FROM pix_transfers WHERE idempotency_key = $1`, t.IdempotencyKey))
	return &existing, fp, err
}

func (s *Store) CreateTransfer(ctx context.Context, t domain.Transfer) (*domain.Transfer, []byte, error) {
	return insertTransfer(ctx, s.pool, t)
}

// CreateRefund inserts a refund and reserves its amount on the original
// transfer, in one transaction with the original row locked, so concurrent
// refunds can never add up to more than was received.
func (s *Store) CreateRefund(ctx context.Context, refund domain.Transfer, check func(original domain.Transfer) error) (*domain.Transfer, []byte, error) {
	var (
		existing *domain.Transfer
		fp       []byte
	)
	err := pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		original, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferCols+` FROM pix_transfers WHERE id = $1 FOR UPDATE`, refund.RefundOf))
		if err != nil {
			return err
		}
		if existing, fp, err = insertTransfer(ctx, tx, refund); err != nil || existing != nil {
			return err // retry of the same refund: nothing more to reserve
		}
		if err := check(original); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE pix_transfers SET refunded_amount = refunded_amount + $2 WHERE id = $1`,
			original.ID, refund.Amount.Minor())
		return err
	})
	return existing, fp, err
}

func (s *Store) InsertQRCode(ctx context.Context, q domain.QRCode) error {
	var amount *int64
	if q.Amount != nil {
		v := q.Amount.Minor()
		amount = &v
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO qr_codes (txid, account_id, key, amount, description, one_time, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, q.TxID, q.AccountID, q.Key, amount, q.Description, q.OneTime, q.ExpiresAt)
	return err
}

func (s *Store) QRCode(ctx context.Context, txid string) (domain.QRCode, error) {
	var (
		q      domain.QRCode
		amount *int64
		paid   *uuid.UUID
	)
	err := s.pool.QueryRow(ctx, `SELECT txid, account_id, key, amount, description, one_time, expires_at, paid_transfer_id, created_at
		FROM qr_codes WHERE txid = $1`, txid).
		Scan(&q.TxID, &q.AccountID, &q.Key, &amount, &q.Description, &q.OneTime, &q.ExpiresAt, &paid, &q.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return q, fmt.Errorf("%w: QR code", domain.ErrNotFound)
	}
	if err != nil {
		return q, err
	}
	if amount != nil {
		m, _ := money.New(*amount, money.BRL)
		q.Amount = &m
	}
	if paid != nil {
		q.PaidTransferID = *paid
	}
	return q, nil
}

// ClaimQRCode marks a one-time charge as being paid by transfer. Claiming
// again with the same transfer succeeds (retries); with another one it fails.
func (s *Store) ClaimQRCode(ctx context.Context, txid string, transfer uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `UPDATE qr_codes SET paid_transfer_id = $2
		WHERE txid = $1 AND (NOT one_time OR paid_transfer_id IS NULL OR paid_transfer_id = $2)
		  AND (expires_at IS NULL OR expires_at > now())`, txid, transfer)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrQRUnavailable
	}
	return nil
}

func (s *Store) Transfer(ctx context.Context, id uuid.UUID) (domain.Transfer, error) {
	return scanTransfer(s.pool.QueryRow(ctx, `SELECT `+transferCols+` FROM pix_transfers WHERE id = $1`, id))
}

func (s *Store) TransferByKey(ctx context.Context, key string) (domain.Transfer, error) {
	return scanTransfer(s.pool.QueryRow(ctx, `SELECT `+transferCols+` FROM pix_transfers WHERE idempotency_key = $1`, key))
}

func (s *Store) Transfers(ctx context.Context, account uuid.UUID, direction domain.Direction, beforeAt time.Time, beforeID uuid.UUID, limit int) ([]domain.Transfer, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+transferCols+` FROM pix_transfers
		WHERE account_id = $1 AND ($2 = '' OR direction = $2)
		  AND ($3::TIMESTAMPTZ IS NULL OR (created_at, id) < ($3, $4))
		ORDER BY created_at DESC, id DESC LIMIT $5`,
		account, string(direction), nullableTime(beforeAt), beforeID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Transfer, error) { return scanTransfer(r) })
}

func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
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

// SaveTransfer stores the new state. On the first move to a failed state it
// also gives back what the transfer held: a refund's reserved amount and a
// one-time QR code's claim. Doing it here, atomically with the status change,
// means retries can never release twice.
func (s *Store) SaveTransfer(ctx context.Context, t domain.Transfer, events ...outbox.Message) error {
	return s.save(ctx, t, nil, events)
}

func (s *Store) CompleteInternal(ctx context.Context, t, incoming domain.Transfer, events ...outbox.Message) error {
	return s.save(ctx, t, &incoming, events)
}

func (s *Store) save(ctx context.Context, t domain.Transfer, incoming *domain.Transfer, events []outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if incoming != nil {
			if _, _, err := insertTransfer(ctx, tx, *incoming); err != nil {
				return err
			}
		}
		var previous string
		if err := tx.QueryRow(ctx, `SELECT status FROM pix_transfers WHERE id = $1 FOR UPDATE`, t.ID).Scan(&previous); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE pix_transfers SET status = $2, transaction_id = $3, failure_reason = $4, updated_at = now()
		                         WHERE id = $1`, t.ID, t.Status, nullable(t.TransactionID), t.FailureReason)
		if err != nil {
			return err
		}
		failedNow := !domain.Status(previous).Terminal() && (t.Status == domain.StatusFailed || t.Status == domain.StatusRejected)
		if failedNow && t.RefundOf != uuid.Nil {
			if _, err := tx.Exec(ctx, `UPDATE pix_transfers SET refunded_amount = refunded_amount - $2 WHERE id = $1`,
				t.RefundOf, t.Amount.Minor()); err != nil {
				return err
			}
		}
		if failedNow && t.QRTxID != "" {
			if _, err := tx.Exec(ctx, `UPDATE qr_codes SET paid_transfer_id = NULL WHERE txid = $1 AND paid_transfer_id = $2`,
				t.QRTxID, t.ID); err != nil {
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

// ---- contacts ----

func (s *Store) Contacts(ctx context.Context, account uuid.UUID, limit int) ([]domain.Contact, error) {
	rows, err := s.pool.Query(ctx, `
		WITH recent AS (
			SELECT key, (array_agg(counterparty_name ORDER BY created_at DESC))[1] AS name,
			       (array_agg(counterparty_bank ORDER BY created_at DESC))[1] AS bank,
			       max(created_at) AS last_at, count(*) AS times
			  FROM pix_transfers
			 WHERE account_id = $1 AND direction = 'OUTGOING' AND status = 'COMPLETED' AND refund_of IS NULL
			 GROUP BY key),
		fav AS (SELECT * FROM pix_favorites WHERE account_id = $1)
		SELECT COALESCE(r.key, f.key), COALESCE(NULLIF(r.name, ''), f.name, ''), COALESCE(NULLIF(r.bank, ''), f.bank_name, ''),
		       COALESCE(f.nickname, ''), f.key IS NOT NULL, r.last_at, COALESCE(r.times, 0)
		  FROM recent r FULL JOIN fav f ON f.key = r.key
		 ORDER BY f.key IS NOT NULL DESC, r.last_at DESC NULLS LAST, COALESCE(f.nickname, r.name)
		 LIMIT $2`, account, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Contact
	for rows.Next() {
		var c domain.Contact
		if err := rows.Scan(&c.Key, &c.Name, &c.BankName, &c.Nickname, &c.Favorite, &c.LastSentAt, &c.TimesSent); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) SaveFavorite(ctx context.Context, account uuid.UUID, c domain.Contact) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "pix-favorites:"+account.String()); err != nil {
			return err
		}
		var n int
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT count(*), COALESCE(bool_or(key = $2), false) FROM pix_favorites WHERE account_id = $1`, account, c.Key).Scan(&n, &exists); err != nil {
			return err
		}
		if !exists && n >= domain.MaxFavorites {
			return domain.Reject("TOO_MANY_FAVORITES", "at most 50 favorites")
		}
		_, err := tx.Exec(ctx, `INSERT INTO pix_favorites (account_id, key, name, bank_name, nickname) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (account_id, key) DO UPDATE SET name = EXCLUDED.name, bank_name = EXCLUDED.bank_name, nickname = EXCLUDED.nickname`,
			account, c.Key, c.Name, c.BankName, c.Nickname)
		return err
	})
}

func (s *Store) RemoveFavorite(ctx context.Context, account uuid.UUID, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM pix_favorites WHERE account_id = $1 AND key = $2`, account, key)
	return err
}
