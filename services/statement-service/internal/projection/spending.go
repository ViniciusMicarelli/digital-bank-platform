package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/inbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
)

const spendingConsumer = "statement-service.spending"

// Spending categories.
const (
	Food          = "FOOD"
	Transport     = "TRANSPORT"
	Shopping      = "SHOPPING"
	Subscriptions = "SUBSCRIPTIONS"
	Health        = "HEALTH"
	Leisure       = "LEISURE"
	Travel        = "TRAVEL"
	Home          = "HOME"
	Education     = "EDUCATION"
	Bills         = "BILLS"
	Transfers     = "TRANSFERS"
	Other         = "OTHER"
)

var mccCategory = map[int]string{
	5411: Food, 5412: Food, 5422: Food, 5441: Food, 5451: Food, 5462: Food, 5499: Food, 5811: Food, 5812: Food, 5813: Food, 5814: Food,
	4111: Transport, 4121: Transport, 4131: Transport, 4784: Transport, 4789: Transport, 5172: Transport, 5541: Transport, 5542: Transport, 7512: Transport, 7523: Transport,
	4814: Subscriptions, 4816: Subscriptions, 4899: Subscriptions, 5815: Subscriptions, 5816: Subscriptions, 5817: Subscriptions, 5818: Subscriptions, 7372: Subscriptions,
	5122: Health, 5912: Health,
	7832: Leisure, 7841: Leisure, 7922: Leisure, 7929: Leisure, 7932: Leisure, 7933: Leisure, 7941: Leisure, 7991: Leisure, 7992: Leisure,
	7993: Leisure, 7994: Leisure, 7996: Leisure, 7997: Leisure, 7998: Leisure, 7999: Leisure,
	4411: Travel, 4511: Travel, 4722: Travel, 7011: Travel, 7012: Travel,
	4900: Home, 5200: Home, 5211: Home, 5231: Home, 5251: Home, 5261: Home, 5712: Home, 5719: Home, 5722: Home,
	5942: Education, 8211: Education, 8220: Education, 8241: Education, 8244: Education, 8249: Education, 8299: Education,
	5311: Shopping, 5331: Shopping, 5732: Shopping, 5733: Shopping, 5734: Shopping, 5735: Shopping, 5945: Shopping, 5977: Shopping, 5999: Shopping,
}

// Category maps a merchant category code (ISO 18245) to a spending category.
func Category(mcc string) string {
	n, err := strconv.Atoi(mcc)
	if err != nil {
		return Other
	}
	if c, ok := mccCategory[n]; ok {
		return c
	}
	switch {
	case n >= 3000 && n <= 3999: // airlines, car rental, hotels
		return Travel
	case n >= 5300 && n <= 5399, n >= 5600 && n <= 5699, n >= 5940 && n <= 5949:
		return Shopping
	case n >= 8011 && n <= 8099:
		return Health
	}
	return Other
}

type cardEvent struct {
	AuthorizationID string `json:"authorization_id"`
	RefundID        string `json:"refund_id"`
	CardID          string `json:"card_id"`
	AccountID       string `json:"account_id"`
	ClearedMinor    int64  `json:"cleared_minor"`
	AmountMinor     int64  `json:"amount_minor"`
	MerchantName    string `json:"merchant_name"`
	MCC             string `json:"mcc"`
}

// card projects card events: issued cards (to resolve accounts), cleared
// purchases and refunds.
func (p *Projector) card(ctx context.Context, ev outbox.Event) error {
	var c cardEvent
	if err := json.Unmarshal(ev.Payload, &c); err != nil {
		return fmt.Errorf("decode %s: %w", ev.Type, err)
	}
	return inbox.Handle(ctx, p.pool, spendingConsumer, ev, func(tx pgx.Tx) error {
		switch ev.Type {
		case "card.issued":
			_, err := tx.Exec(ctx, `INSERT INTO cards_view (card_id, account_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, c.CardID, c.AccountID)
			return err
		case "card.purchase.cleared":
			account, err := resolveAccount(ctx, tx, c)
			if err != nil || account == "" {
				return err // a card this read model never saw: nothing to place
			}
			category := Category(c.MCC)
			if err := insertSpending(ctx, tx, account, "card:"+c.AuthorizationID, category, c.MerchantName, c.ClearedMinor, ev.OccurredAt); err != nil {
				return err
			}
			// Refunds that arrived first.
			rows, err := tx.Query(ctx, `DELETE FROM pending_refunds WHERE authorization_id = $1 RETURNING refund_id, amount, occurred_at`, c.AuthorizationID)
			if err != nil {
				return err
			}
			type pending struct {
				id     string
				amount int64
				at     time.Time
			}
			list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (pending, error) {
				var p pending
				return p, r.Scan(&p.id, &p.amount, &p.at)
			})
			if err != nil {
				return err
			}
			for _, p := range list {
				if err := insertSpending(ctx, tx, account, "refund:"+p.id, category, c.MerchantName, -p.amount, p.at); err != nil {
					return err
				}
			}
			return nil
		case "card.purchase.refunded":
			var account, category, merchant string
			err := tx.QueryRow(ctx, `SELECT account_id, category, merchant FROM spending WHERE ref = $1`, "card:"+c.AuthorizationID).
				Scan(&account, &category, &merchant)
			if err == pgx.ErrNoRows { // the purchase is not projected yet: keep the refund for it
				_, err := tx.Exec(ctx, `INSERT INTO pending_refunds (refund_id, authorization_id, amount, occurred_at)
					VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, c.RefundID, c.AuthorizationID, c.AmountMinor, ev.OccurredAt)
				return err
			}
			if err != nil {
				return err
			}
			return insertSpending(ctx, tx, account, "refund:"+c.RefundID, category, merchant, -c.AmountMinor, ev.OccurredAt)
		}
		return nil
	})
}

func resolveAccount(ctx context.Context, tx pgx.Tx, c cardEvent) (string, error) {
	if c.AccountID != "" && c.AccountID != uuid.Nil.String() {
		return c.AccountID, nil
	}
	var account string
	err := tx.QueryRow(ctx, `SELECT account_id FROM cards_view WHERE card_id = $1`, c.CardID).Scan(&account)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return account, err
}

func insertSpending(ctx context.Context, tx pgx.Tx, account, ref, category, merchant string, amount int64, at time.Time) error {
	if amount == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO spending (id, account_id, ref, category, merchant, amount, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (account_id, ref) DO NOTHING`,
		uuid.Must(uuid.NewV7()), account, ref, category, merchant, amount, at)
	return err
}

// transactionSpending is the spending side of a completed transaction:
// Pix and transfers sent, bills paid. Card purchases come from card events;
// invoice payments and savings are not spending.
func transactionSpending(t transactionEvent) (category, merchant string, ok bool) {
	switch t.Type {
	case "PIX_OUT", "INTERNAL_TRANSFER":
		return Transfers, t.Metadata["counterparty_name"], true
	case "BOLETO_PAYMENT":
		return Bills, t.Metadata["counterparty_name"], true
	}
	return "", "", false
}

// Brasilia is the bank's month boundary (no daylight saving since 2019).
var Brasilia = time.FixedZone("BRT", -3*60*60)

// CategoryAmount, MerchantAmount and Spending are a month's summary.
type CategoryAmount struct {
	Category string
	Amount   int64
	Count    int
}

type MerchantAmount struct {
	Name     string
	Category string
	Amount   int64
	Count    int
}

type Spending struct {
	Month         string
	Total         int64
	PreviousTotal int64
	Categories    []CategoryAmount
	Merchants     []MerchantAmount
}

// ParseMonth reads YYYY-MM (or now's month when empty) as a Brasília month.
func ParseMonth(month string, now time.Time) (time.Time, error) {
	if month == "" {
		y, m, _ := now.In(Brasilia).Date()
		return time.Date(y, m, 1, 0, 0, 0, 0, Brasilia), nil
	}
	t, err := time.ParseInLocation("2006-01", month, Brasilia)
	if err != nil {
		return time.Time{}, fmt.Errorf("month must be YYYY-MM")
	}
	return t, nil
}

// Spending summarizes where an account's money went in the month starting at start.
func (p *Projector) Spending(ctx context.Context, account uuid.UUID, start time.Time) (Spending, error) {
	end := start.AddDate(0, 1, 0)
	out := Spending{Month: start.Format("2006-01")}
	rows, err := p.pool.Query(ctx, `SELECT category, sum(amount), count(*) FILTER (WHERE amount > 0) FROM spending
		WHERE account_id = $1 AND occurred_at >= $2 AND occurred_at < $3 GROUP BY category`, account, start, end)
	if err != nil {
		return out, err
	}
	out.Categories, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (CategoryAmount, error) {
		var c CategoryAmount
		return c, r.Scan(&c.Category, &c.Amount, &c.Count)
	})
	if err != nil {
		return out, err
	}
	kept := out.Categories[:0]
	for _, c := range out.Categories {
		if c.Amount > 0 { // fully refunded categories drop out
			out.Total += c.Amount
			kept = append(kept, c)
		}
	}
	out.Categories = kept
	sort.Slice(out.Categories, func(i, j int) bool { return out.Categories[i].Amount > out.Categories[j].Amount })
	if err := p.pool.QueryRow(ctx, `SELECT COALESCE(sum(amount), 0) FROM spending WHERE account_id = $1 AND occurred_at >= $2 AND occurred_at < $3`,
		account, start.AddDate(0, -1, 0), start).Scan(&out.PreviousTotal); err != nil {
		return out, err
	}
	rows, err = p.pool.Query(ctx, `SELECT merchant, min(category), sum(amount), count(*) FILTER (WHERE amount > 0) FROM spending
		WHERE account_id = $1 AND occurred_at >= $2 AND occurred_at < $3 AND category <> 'TRANSFERS' AND merchant <> ''
		GROUP BY merchant HAVING sum(amount) > 0 ORDER BY sum(amount) DESC LIMIT 5`, account, start, end)
	if err != nil {
		return out, err
	}
	out.Merchants, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (MerchantAmount, error) {
		var m MerchantAmount
		return m, r.Scan(&m.Name, &m.Category, &m.Amount, &m.Count)
	})
	return out, err
}
