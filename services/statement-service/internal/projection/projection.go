// Package projection builds the customer feed from events and serves it.
//
// The read model is disposable: dropping the tables and replaying the topics
// from the beginning (new consumer group) rebuilds it.
package projection

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/inbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
)

const consumer = "statement-service.feed"

// ErrInvalidCursor is returned for a malformed page token.
var ErrInvalidCursor = errors.New("invalid page token")

type Projector struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Projector { return &Projector{pool: pool} }

// Handle routes an event to its projection. Unknown types are ignored, so
// new event types never break this consumer.
func (p *Projector) Handle(ctx context.Context, ev outbox.Event) error {
	switch {
	case strings.HasPrefix(ev.Type, "account."):
		return p.account(ctx, ev)
	case ev.Type == "transaction.completed":
		return p.transaction(ctx, ev)
	}
	return nil
}

type accountEvent struct {
	AccountID  string `json:"account_id"`
	HolderName string `json:"holder_name"`
	Branch     string `json:"branch"`
	Number     string `json:"number"`
	Status     string `json:"status"`
}

func (p *Projector) account(ctx context.Context, ev outbox.Event) error {
	var a accountEvent
	if err := json.Unmarshal(ev.Payload, &a); err != nil {
		return fmt.Errorf("decode %s: %w", ev.Type, err)
	}
	if a.HolderName == "" { // e.g. account.limits.changed
		return nil
	}
	return inbox.Handle(ctx, p.pool, consumer, ev, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO accounts_view (account_id, holder_name, branch, number, status)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (account_id) DO UPDATE
			   SET holder_name = EXCLUDED.holder_name, status = EXCLUDED.status, updated_at = now()`,
			a.AccountID, a.HolderName, a.Branch, a.Number, a.Status)
		return err
	})
}

type transactionEvent struct {
	TransactionID string            `json:"transaction_id"`
	Type          string            `json:"type"`
	FromAccountID string            `json:"from_account_id"`
	ToAccountID   string            `json:"to_account_id"`
	AmountMinor   int64             `json:"amount_minor"`
	Currency      string            `json:"currency"`
	Description   string            `json:"description"`
	Metadata      map[string]string `json:"metadata"`
	OccurredAt    time.Time         `json:"occurred_at"`
}

type item struct {
	account, counterpartyAccount string
	direction                    string
	counterpartyName             *string
}

// sides decides which customer accounts see the transaction. Internal bank
// accounts (Pix settlement) never get feed items.
func sides(t transactionEvent) []item {
	var name *string
	if n := t.Metadata["counterparty_name"]; n != "" {
		name = &n
	}
	switch t.Type {
	case "INTERNAL_TRANSFER":
		// A same-bank Pix is an internal transfer with Pix metadata; the
		// receiver's view of the counterparty is the sender's account.
		return []item{
			{account: t.FromAccountID, counterpartyAccount: t.ToAccountID, direction: "DEBIT", counterpartyName: name},
			{account: t.ToAccountID, counterpartyAccount: t.FromAccountID, direction: "CREDIT"},
		}
	case "PIX_OUT":
		return []item{{account: t.FromAccountID, direction: "DEBIT", counterpartyName: name}}
	case "PIX_IN", "CARD_REFUND", "SAVINGS_WITHDRAWAL":
		return []item{{account: t.ToAccountID, direction: "CREDIT", counterpartyName: name}}
	case "SAVINGS_DEPOSIT":
		return []item{{account: t.FromAccountID, direction: "DEBIT", counterpartyName: name}}
	case "DEBIT_CARD_PURCHASE", "CREDIT_INVOICE_PAYMENT", "BOLETO_PAYMENT":
		return []item{{account: t.FromAccountID, direction: "DEBIT", counterpartyName: name}}
	}
	return nil
}

func (p *Projector) transaction(ctx context.Context, ev outbox.Event) error {
	var t transactionEvent
	if err := json.Unmarshal(ev.Payload, &t); err != nil {
		return fmt.Errorf("decode %s: %w", ev.Type, err)
	}
	if t.OccurredAt.IsZero() {
		t.OccurredAt = ev.OccurredAt
	}
	typ := t.Type
	switch {
	case t.Metadata["refund_of"] != "":
		typ = "PIX_REFUND"
	case t.Type == "INTERNAL_TRANSFER" && t.Metadata["end_to_end_id"] != "":
		typ = "PIX" // same-bank Pix
	}
	return inbox.Handle(ctx, p.pool, consumer, ev, func(tx pgx.Tx) error {
		for _, s := range sides(t) {
			var cpAccount *string
			if s.counterpartyAccount != "" {
				cpAccount = &s.counterpartyAccount
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO feed_items (id, account_id, transaction_id, type, direction, amount, currency,
				                        counterparty_name, counterparty_account_id, description, end_to_end_id, occurred_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
				ON CONFLICT (account_id, transaction_id) DO NOTHING`,
				uuid.Must(uuid.NewV7()), s.account, t.TransactionID, typ, s.direction, t.AmountMinor, t.Currency,
				s.counterpartyName, cpAccount, t.Description, t.Metadata["end_to_end_id"], t.OccurredAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Item is a feed entry as served to clients.
type Item struct {
	ID               uuid.UUID
	AccountID        uuid.UUID
	TransactionID    uuid.UUID
	Type             string
	Direction        string
	AmountMinor      int64
	Currency         string
	CounterpartyName string
	Description      string
	EndToEndID       string
	OccurredAt       time.Time
}

type cursor struct {
	At time.Time `json:"t"`
	ID uuid.UUID `json:"i"`
}

// Feed returns up to limit items for account, newest first, after the cursor.
func (p *Projector) Feed(ctx context.Context, account uuid.UUID, pageToken string, limit int) ([]Item, string, error) {
	var after *cursor
	if pageToken != "" {
		raw, err := base64.RawURLEncoding.DecodeString(pageToken)
		if err != nil {
			return nil, "", ErrInvalidCursor
		}
		after = &cursor{}
		if err := json.Unmarshal(raw, after); err != nil {
			return nil, "", ErrInvalidCursor
		}
	}
	args := []any{account, limit + 1}
	where := ""
	if after != nil {
		where = `AND (f.occurred_at, f.id) < ($3, $4)`
		args = append(args, after.At, after.ID)
	}
	rows, err := p.pool.Query(ctx, `
		SELECT f.id, f.account_id, f.transaction_id, f.type, f.direction, f.amount, f.currency,
		       COALESCE(f.counterparty_name, a.holder_name, ''), f.description, f.end_to_end_id, f.occurred_at
		  FROM feed_items f
		  LEFT JOIN accounts_view a ON a.account_id = f.counterparty_account_id
		 WHERE f.account_id = $1 `+where+`
		 ORDER BY f.occurred_at DESC, f.id DESC
		 LIMIT $2`, args...)
	if err != nil {
		return nil, "", err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Item, error) {
		var it Item
		err := r.Scan(&it.ID, &it.AccountID, &it.TransactionID, &it.Type, &it.Direction, &it.AmountMinor,
			&it.Currency, &it.CounterpartyName, &it.Description, &it.EndToEndID, &it.OccurredAt)
		return it, err
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		raw, _ := json.Marshal(cursor{At: last.OccurredAt, ID: last.ID})
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	return items, next, nil
}
