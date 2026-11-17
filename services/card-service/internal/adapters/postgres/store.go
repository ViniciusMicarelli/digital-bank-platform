// Package postgres implements the card store and the vault tables.
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
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/ports"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

const cardCols = `id, idempotency_key, account_id, kind, status, status_reason, last4, expiry_month, expiry_year,
	holder_name, online, international, contactless, created_at`

func scanCard(row pgx.Row) (domain.Card, error) {
	var (
		c            domain.Card
		kind, status string
	)
	err := row.Scan(&c.ID, &c.IdempotencyKey, &c.AccountID, &kind, &status, &c.StatusReason, &c.Last4,
		&c.ExpiryMonth, &c.ExpiryYear, &c.HolderName, &c.Controls.Online, &c.Controls.International,
		&c.Controls.Contactless, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, fmt.Errorf("%w: card", domain.ErrNotFound)
	}
	c.Kind, c.Status = domain.Kind(kind), domain.Status(status)
	return c, err
}

func (s *Store) InsertCard(ctx context.Context, c domain.Card, panEnc, panIndex []byte, ev outbox.Message) (*domain.Card, error) {
	var existing *domain.Card
	err := pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO cards (id, idempotency_key, account_id, kind, status, last4, expiry_month, expiry_year,
			holder_name, online, international, contactless) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (idempotency_key) DO NOTHING`,
			c.ID, c.IdempotencyKey, c.AccountID, c.Kind, c.Status, c.Last4, c.ExpiryMonth, c.ExpiryYear,
			c.HolderName, c.Controls.Online, c.Controls.International, c.Controls.Contactless)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			stored, err := scanCard(tx.QueryRow(ctx, `SELECT `+cardCols+` FROM cards WHERE idempotency_key = $1`, c.IdempotencyKey))
			existing = &stored
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO card_vault (card_id, pan_enc, pan_index) VALUES ($1, $2, $3)`,
			c.ID, panEnc, panIndex); err != nil {
			return err
		}
		return outbox.Insert(ctx, tx, ev)
	})
	return existing, err
}

func (s *Store) Card(ctx context.Context, id uuid.UUID) (domain.Card, error) {
	return scanCard(s.pool.QueryRow(ctx, `SELECT `+cardCols+` FROM cards WHERE id = $1`, id))
}

func (s *Store) CardsByAccount(ctx context.Context, account uuid.UUID) ([]domain.Card, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+cardCols+` FROM cards WHERE account_id = $1 ORDER BY created_at`, account)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Card, error) { return scanCard(r) })
}

func (s *Store) CardByPANIndex(ctx context.Context, index []byte) (domain.Card, error) {
	return scanCard(s.pool.QueryRow(ctx, `SELECT `+cardCols+` FROM cards
		WHERE id = (SELECT card_id FROM card_vault WHERE pan_index = $1)`, index))
}

func (s *Store) PANEnc(ctx context.Context, card uuid.UUID) ([]byte, error) {
	var enc []byte
	err := s.pool.QueryRow(ctx, `SELECT pan_enc FROM card_vault WHERE card_id = $1`, card).Scan(&enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: card", domain.ErrNotFound)
	}
	return enc, err
}

func (s *Store) SaveCard(ctx context.Context, c domain.Card, events ...outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE cards SET status = $2, status_reason = $3, online = $4, international = $5,
			contactless = $6, updated_at = now() WHERE id = $1`,
			c.ID, c.Status, c.StatusReason, c.Controls.Online, c.Controls.International, c.Controls.Contactless); err != nil {
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

const authCols = `id, network_reference, card_id, status, response_code, reason, amount, currency, cleared_amount,
	merchant_name, mcc, country, channel, transaction_id, created_at`

func scanAuth(row pgx.Row) (domain.Authorization, error) {
	var (
		a                         domain.Authorization
		card, txID                *uuid.UUID
		status, currency, channel string
		amount                    int64
	)
	err := row.Scan(&a.ID, &a.NetworkReference, &card, &status, &a.Code, &a.Reason, &amount, &currency, &a.Cleared,
		&a.Purchase.MerchantName, &a.Purchase.MCC, &a.Purchase.Country, &channel, &txID, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fmt.Errorf("%w: authorization", domain.ErrNotFound)
	}
	if err != nil {
		return a, err
	}
	a.Status, a.Purchase.Channel, a.Purchase.NetworkReference = domain.AuthStatus(status), domain.Channel(channel), a.NetworkReference
	if card != nil {
		a.CardID = *card
	}
	if txID != nil {
		a.TransactionID = *txID
	}
	a.Purchase.Amount, err = money.New(amount, money.Currency(currency))
	return a, err
}

func nullable(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func (s *Store) ClaimAuthorization(ctx context.Context, a domain.Authorization) (*domain.Authorization, error) {
	p := a.Purchase
	tag, err := s.pool.Exec(ctx, `INSERT INTO authorizations (id, network_reference, card_id, status, response_code, reason,
		amount, currency, merchant_name, mcc, country, channel)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) ON CONFLICT (network_reference) DO NOTHING`,
		a.ID, a.NetworkReference, nullable(a.CardID), a.Status, a.Code, a.Reason, p.Amount.Minor(), string(p.Amount.Currency()),
		p.MerchantName, p.MCC, p.Country, p.Channel)
	if err != nil || tag.RowsAffected() == 1 {
		return nil, err
	}
	existing, err := scanAuth(s.pool.QueryRow(ctx, `SELECT `+authCols+` FROM authorizations WHERE network_reference = $1`, a.NetworkReference))
	return &existing, err
}

func (s *Store) Authorization(ctx context.Context, id uuid.UUID) (domain.Authorization, error) {
	return scanAuth(s.pool.QueryRow(ctx, `SELECT `+authCols+` FROM authorizations WHERE id = $1`, id))
}

func (s *Store) SaveAuthorization(ctx context.Context, a domain.Authorization, from domain.AuthStatus, events ...outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE authorizations SET status = $3, response_code = $4, reason = $5,
			cleared_amount = $6, transaction_id = COALESCE($7, transaction_id), updated_at = now()
			WHERE id = $1 AND status = $2`,
			a.ID, from, a.Status, a.Code, a.Reason, a.Cleared, nullable(a.TransactionID))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w: authorization %s is no longer %s", domain.ErrInvalidState, a.ID, from)
		}
		for _, ev := range events {
			if err := outbox.Insert(ctx, tx, ev); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) PendingReversals(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM authorizations WHERE status = 'PENDING_REVERSAL' AND updated_at < $1
		ORDER BY updated_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// HandleAccountEvent mirrors the account status on its cards: blocked
// accounts block their cards, unblocking restores only those, closed
// accounts cancel them. Runs once per event (inbox).
func (s *Store) HandleAccountEvent(ctx context.Context, ev outbox.Event) error {
	var sql string
	switch ev.Type {
	case "account.blocked":
		sql = `UPDATE cards SET status = 'BLOCKED', status_reason = '` + domain.ReasonAccountBlocked + `', updated_at = now()
			WHERE account_id = $1 AND status = 'ACTIVE'`
	case "account.unblocked":
		sql = `UPDATE cards SET status = 'ACTIVE', status_reason = '', updated_at = now()
			WHERE account_id = $1 AND status = 'BLOCKED' AND status_reason = '` + domain.ReasonAccountBlocked + `'`
	case "account.closed":
		sql = `UPDATE cards SET status = 'CANCELLED', status_reason = 'ACCOUNT_CLOSED', updated_at = now()
			WHERE account_id = $1 AND status <> 'CANCELLED'`
	default:
		return nil
	}
	var p struct {
		AccountID string `json:"account_id"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return fmt.Errorf("decode %s: %w", ev.Type, err)
	}
	return inbox.Handle(ctx, s.pool, "card-service.accounts", ev, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, p.AccountID)
		return err
	})
}
