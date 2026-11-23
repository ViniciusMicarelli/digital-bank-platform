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
	holder_name, online, international, contactless, created_at, valid_until, monthly_limit`

func scanCard(row pgx.Row) (domain.Card, error) {
	var (
		c            domain.Card
		kind, status string
	)
	err := row.Scan(&c.ID, &c.IdempotencyKey, &c.AccountID, &kind, &status, &c.StatusReason, &c.Last4,
		&c.ExpiryMonth, &c.ExpiryYear, &c.HolderName, &c.Controls.Online, &c.Controls.International,
		&c.Controls.Contactless, &c.CreatedAt, &c.ValidUntil, &c.MonthlyLimit)
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
			holder_name, online, international, contactless, valid_until) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (idempotency_key) DO NOTHING`,
			c.ID, c.IdempotencyKey, c.AccountID, c.Kind, c.Status, c.Last4, c.ExpiryMonth, c.ExpiryYear,
			c.HolderName, c.Controls.Online, c.Controls.International, c.Controls.Contactless, c.ValidUntil)
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
			contactless = $6, monthly_limit = $7, updated_at = now() WHERE id = $1`,
			c.ID, c.Status, c.StatusReason, c.Controls.Online, c.Controls.International, c.Controls.Contactless, c.MonthlyLimit); err != nil {
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

const tokenCols = `id, card_id, wallet, device_name, status, last4, expiry_month, expiry_year, created_at`

func scanToken(row pgx.Row) (domain.Token, error) {
	var (
		t              domain.Token
		wallet, status string
	)
	err := row.Scan(&t.ID, &t.CardID, &wallet, &t.DeviceName, &status, &t.Last4, &t.ExpiryMonth, &t.ExpiryYear, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, fmt.Errorf("%w: token", domain.ErrNotFound)
	}
	t.Wallet, t.Status = domain.Wallet(wallet), domain.TokenStatus(status)
	return t, err
}

func (s *Store) InsertToken(ctx context.Context, t domain.Token, panEnc, panIndex []byte, ev outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO card_tokens (id, card_id, wallet, device_name, status, last4, expiry_month,
			expiry_year, pan_enc, pan_index) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			t.ID, t.CardID, t.Wallet, t.DeviceName, t.Status, t.Last4, t.ExpiryMonth, t.ExpiryYear, panEnc, panIndex); err != nil {
			return err
		}
		return outbox.Insert(ctx, tx, ev)
	})
}

func (s *Store) Tokens(ctx context.Context, card uuid.UUID) ([]domain.Token, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+tokenCols+` FROM card_tokens WHERE card_id = $1 AND status <> 'DELETED' ORDER BY created_at`, card)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Token, error) { return scanToken(r) })
}

func (s *Store) Token(ctx context.Context, id uuid.UUID) (domain.Token, error) {
	return scanToken(s.pool.QueryRow(ctx, `SELECT `+tokenCols+` FROM card_tokens WHERE id = $1`, id))
}

func (s *Store) TokenByPANIndex(ctx context.Context, index []byte) (domain.Token, error) {
	return scanToken(s.pool.QueryRow(ctx, `SELECT `+tokenCols+` FROM card_tokens WHERE pan_index = $1`, index))
}

func (s *Store) SaveToken(ctx context.Context, t domain.Token, ev outbox.Message) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE card_tokens SET status = $2, updated_at = now() WHERE id = $1`, t.ID, t.Status); err != nil {
			return err
		}
		return outbox.Insert(ctx, tx, ev)
	})
}

const authCols = `id, network_reference, card_id, token_id, status, response_code, reason, amount, currency, cleared_amount,
	merchant_name, mcc, country, channel, transaction_id, created_at, function, refunded_amount, installments`

func scanAuth(row pgx.Row) (domain.Authorization, error) {
	var (
		a                         domain.Authorization
		card, token, txID         *uuid.UUID
		status, currency, channel string
		function                  string
		amount                    int64
	)
	err := row.Scan(&a.ID, &a.NetworkReference, &card, &token, &status, &a.Code, &a.Reason, &amount, &currency, &a.Cleared,
		&a.Purchase.MerchantName, &a.Purchase.MCC, &a.Purchase.Country, &channel, &txID, &a.CreatedAt, &function, &a.Refunded, &a.Purchase.Installments)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fmt.Errorf("%w: authorization", domain.ErrNotFound)
	}
	if err != nil {
		return a, err
	}
	a.Status, a.Purchase.Channel, a.Purchase.NetworkReference = domain.AuthStatus(status), domain.Channel(channel), a.NetworkReference
	a.Purchase.Function = domain.Function(function)
	if card != nil {
		a.CardID = *card
	}
	if token != nil {
		a.TokenID = *token
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
	tag, err := s.pool.Exec(ctx, `INSERT INTO authorizations (id, network_reference, card_id, token_id, status, response_code, reason,
		amount, currency, merchant_name, mcc, country, channel, function, installments, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, COALESCE($16, now())) ON CONFLICT (network_reference) DO NOTHING`,
		a.ID, a.NetworkReference, nullable(a.CardID), nullable(a.TokenID), a.Status, a.Code, a.Reason, p.Amount.Minor(), string(p.Amount.Currency()),
		p.MerchantName, p.MCC, p.Country, p.Channel, p.Function, max(p.Installments, 1), nullTime(a.CreatedAt))
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

func (s *Store) AuthorizationsByCard(ctx context.Context, card uuid.UUID, limit int) ([]domain.Authorization, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT `+authCols+` FROM authorizations WHERE card_id = $1 AND status <> 'PENDING_REVERSAL'
		ORDER BY created_at DESC LIMIT $2`, card, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Authorization
	for rows.Next() {
		a, err := scanAuth(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- refunds and disputes ----

type unit struct {
	ctx context.Context
	tx  pgx.Tx
}

func (s *Store) InTx(ctx context.Context, fn func(ports.Tx) error) error {
	return pg.InTx(ctx, s.pool, func(tx pgx.Tx) error { return fn(&unit{ctx: ctx, tx: tx}) })
}

func (u *unit) exec(sql string, args ...any) error {
	_, err := u.tx.Exec(u.ctx, sql, args...)
	return err
}

func (u *unit) LockAuthorization(id uuid.UUID) (domain.Authorization, error) {
	a, err := scanAuth(u.tx.QueryRow(u.ctx, `SELECT `+authCols+` FROM authorizations WHERE id = $1 FOR UPDATE`, id))
	if err != nil || a.CardID == uuid.Nil {
		return a, err
	}
	err = u.tx.QueryRow(u.ctx, `SELECT account_id FROM cards WHERE id = $1`, a.CardID).Scan(&a.AccountID)
	return a, err
}

func (u *unit) SetRefunded(id uuid.UUID, refunded int64) error {
	return u.exec(`UPDATE authorizations SET refunded_amount = $2, updated_at = now() WHERE id = $1`, id, refunded)
}

const disputeCols = `d.id, d.authorization_id, d.card_id, d.account_id, d.reason, d.amount, d.description, d.status, d.decided_by,
	d.decision_reason, d.created_at, d.decided_at, a.merchant_name, a.created_at`

const disputeFrom = ` FROM disputes d JOIN authorizations a ON a.id = d.authorization_id`

func scanDispute(row pgx.Row) (domain.Dispute, error) {
	var (
		d              domain.Dispute
		reason, status string
	)
	err := row.Scan(&d.ID, &d.AuthorizationID, &d.CardID, &d.AccountID, &reason, &d.Amount, &d.Description, &status, &d.DecidedBy,
		&d.DecisionReason, &d.CreatedAt, &d.DecidedAt, &d.MerchantName, &d.PurchasedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, fmt.Errorf("%w: dispute", domain.ErrNotFound)
	}
	d.Reason, d.Status = domain.DisputeReason(reason), domain.DisputeStatus(status)
	return d, err
}

func collectDisputes(rows pgx.Rows, err error) ([]domain.Dispute, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Dispute
	for rows.Next() {
		d, err := scanDispute(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (u *unit) OpenDispute(authorization uuid.UUID) (*domain.Dispute, error) {
	d, err := scanDispute(u.tx.QueryRow(u.ctx, `SELECT `+disputeCols+disputeFrom+` WHERE d.authorization_id = $1 AND d.status = 'OPEN'`, authorization))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return &d, err
}

func (u *unit) InsertDispute(d domain.Dispute) error {
	err := u.exec(`INSERT INTO disputes (id, authorization_id, card_id, account_id, reason, amount, description, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		d.ID, d.AuthorizationID, d.CardID, d.AccountID, d.Reason, d.Amount, d.Description, d.Status, d.CreatedAt)
	if pg.IsUniqueViolation(err, "disputes_one_open") {
		return domain.Refuse("DISPUTE_ALREADY_OPEN", "this purchase is already being disputed")
	}
	return err
}

func (u *unit) LockDispute(id uuid.UUID) (domain.Dispute, error) {
	return scanDispute(u.tx.QueryRow(u.ctx, `SELECT `+disputeCols+disputeFrom+` WHERE d.id = $1 FOR UPDATE OF d`, id))
}

func (u *unit) SaveDispute(d domain.Dispute) error {
	return u.exec(`UPDATE disputes SET status = $2, decided_by = $3, decision_reason = $4, decided_at = $5 WHERE id = $1`,
		d.ID, d.Status, d.DecidedBy, d.DecisionReason, d.DecidedAt)
}

const refundCols = `r.id, r.authorization_id, r.account_id, r.amount, r.source, r.reference, r.status, r.failure_reason, r.created_at,
	a.function, a.merchant_name`

const refundFrom = ` FROM refunds r JOIN authorizations a ON a.id = r.authorization_id`

func scanRefund(row pgx.Row) (domain.Refund, error) {
	var (
		r                        domain.Refund
		source, status, function string
	)
	err := row.Scan(&r.ID, &r.AuthorizationID, &r.AccountID, &r.Amount, &source, &r.Reference, &status, &r.FailureReason, &r.CreatedAt,
		&function, &r.MerchantName)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, fmt.Errorf("%w: refund", domain.ErrNotFound)
	}
	r.Source, r.Status, r.Function = domain.RefundSource(source), domain.RefundStatus(status), domain.Function(function)
	return r, err
}

func (u *unit) RefundByReference(authorization uuid.UUID, reference string) (*domain.Refund, error) {
	r, err := scanRefund(u.tx.QueryRow(u.ctx, `SELECT `+refundCols+refundFrom+` WHERE r.authorization_id = $1 AND r.reference = $2`,
		authorization, reference))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return &r, err
}

func (u *unit) InsertRefund(r domain.Refund) error {
	return u.exec(`INSERT INTO refunds (id, authorization_id, account_id, amount, source, reference, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, r.ID, r.AuthorizationID, r.AccountID, r.Amount, r.Source, r.Reference, r.Status, r.CreatedAt)
}

func (u *unit) LockRefund(id uuid.UUID) (domain.Refund, error) {
	return scanRefund(u.tx.QueryRow(u.ctx, `SELECT `+refundCols+refundFrom+` WHERE r.id = $1 FOR UPDATE OF r`, id))
}

func (u *unit) SaveRefund(r domain.Refund) error {
	return u.exec(`UPDATE refunds SET status = $2, failure_reason = $3, updated_at = now() WHERE id = $1`, r.ID, r.Status, r.FailureReason)
}

func (u *unit) LockCard(id uuid.UUID) (domain.Card, error) {
	return scanCard(u.tx.QueryRow(u.ctx, `SELECT `+cardCols+` FROM cards WHERE id = $1 FOR UPDATE`, id))
}

func (u *unit) SaveCard(c domain.Card) error {
	return u.exec(`UPDATE cards SET status = $2, status_reason = $3, updated_at = now() WHERE id = $1`, c.ID, c.Status, c.StatusReason)
}

func (u *unit) Publish(m outbox.Message) error { return outbox.Insert(u.ctx, u.tx, m) }

func (s *Store) Dispute(ctx context.Context, id uuid.UUID) (domain.Dispute, error) {
	return scanDispute(s.pool.QueryRow(ctx, `SELECT `+disputeCols+disputeFrom+` WHERE d.id = $1`, id))
}

func (s *Store) Disputes(ctx context.Context, account uuid.UUID, status domain.DisputeStatus, limit int) ([]domain.Dispute, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var acc *uuid.UUID
	if account != uuid.Nil {
		acc = &account
	}
	return collectDisputes(s.pool.Query(ctx, `SELECT `+disputeCols+disputeFrom+`
		WHERE ($1::uuid IS NULL OR d.account_id = $1) AND ($2 = '' OR d.status = $2) ORDER BY d.created_at DESC LIMIT $3`,
		acc, string(status), limit))
}

func (s *Store) LatestDisputes(ctx context.Context, authorizations []uuid.UUID) (map[uuid.UUID]domain.Dispute, error) {
	list, err := collectDisputes(s.pool.Query(ctx, `SELECT DISTINCT ON (d.authorization_id) `+disputeCols+disputeFrom+`
		WHERE d.authorization_id = ANY($1) ORDER BY d.authorization_id, d.created_at DESC`, authorizations))
	out := map[uuid.UUID]domain.Dispute{}
	for _, d := range list {
		out[d.AuthorizationID] = d
	}
	return out, err
}

func (s *Store) PendingRefunds(ctx context.Context, before time.Time, limit int) ([]domain.Refund, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+refundCols+refundFrom+` WHERE r.status = 'PENDING' AND r.updated_at < $1
		ORDER BY r.updated_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Refund
	for rows.Next() {
		r, err := scanRefund(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UseTemporary claims a temporary card for its one purchase: it is cancelled
// (USED) only if still active, so two concurrent purchases cannot both use it.
func (s *Store) UseTemporary(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE cards SET status = 'CANCELLED', status_reason = 'USED', updated_at = now()
		WHERE id = $1 AND kind = 'TEMPORARY' AND status = 'ACTIVE'`, id)
	return tag.RowsAffected() == 1, err
}

// ReleaseTemporary gives the card back when its purchase was declined after the claim.
func (s *Store) ReleaseTemporary(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE cards SET status = 'ACTIVE', status_reason = '', updated_at = now()
		WHERE id = $1 AND kind = 'TEMPORARY' AND status = 'CANCELLED' AND status_reason = 'USED'`, id)
	return err
}

// MonthUsed is what the card spent since since: approved and cleared
// purchases (cleared ones at the cleared amount, net of refunds) and the
// ones still being decided or cleaned up, except one (the purchase being
// decided). Every authorization is stored before it is decided, so two
// purchases at once always see each other: the limit can only be enforced
// too strictly, never exceeded.
func (s *Store) MonthUsed(ctx context.Context, card uuid.UUID, since time.Time, except uuid.UUID) (int64, error) {
	var used int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(sum(CASE WHEN status = 'CLEARED' THEN cleared_amount - refunded_amount ELSE amount END), 0)
		FROM authorizations WHERE card_id = $1 AND created_at >= $2 AND id <> $3
		AND status IN ('APPROVED', 'CLEARED', 'PENDING_REVERSAL')`, card, since, except).Scan(&used)
	return used, err
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
