package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
)

// Total is the budget on all spending of the month.
const Total = "TOTAL"

// BudgetCategories are what a budget can be set on.
var BudgetCategories = map[string]bool{Total: true, Food: true, Transport: true, Shopping: true, Subscriptions: true, Health: true,
	Leisure: true, Travel: true, Home: true, Education: true, Bills: true, Transfers: true, Other: true}

// MaxBudget keeps budgets in a sane range (R$ 1.000.000,00).
const MaxBudget = 1_000_000_00

// Thresholds are the shares of a budget that send an alert, once a month each.
var Thresholds = []int{100, 80}

// ErrInvalidBudget is a budget on an unknown category or out of range.
var ErrInvalidBudget = errors.New("invalid budget")

// Budget is a monthly budget with what was spent against it.
type Budget struct {
	Category string
	Amount   int64
	Spent    int64
}

// SetBudget sets (or, with amount 0, removes) the account's monthly budget
// on a category.
func (p *Projector) SetBudget(ctx context.Context, account uuid.UUID, category string, amount int64) error {
	if !BudgetCategories[category] || amount < 0 || amount > MaxBudget {
		return fmt.Errorf("%w: a known category and an amount up to R$ 1.000.000,00", ErrInvalidBudget)
	}
	if amount == 0 {
		_, err := p.pool.Exec(ctx, `DELETE FROM budgets WHERE account_id = $1 AND category = $2`, account, category)
		return err
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO budgets (account_id, category, amount) VALUES ($1, $2, $3)
		ON CONFLICT (account_id, category) DO UPDATE SET amount = EXCLUDED.amount, updated_at = now()`, account, category, amount)
	return err
}

// Budgets lists the account's budgets with what was spent in the month
// starting at start, TOTAL first.
func (p *Projector) Budgets(ctx context.Context, account uuid.UUID, start time.Time) ([]Budget, error) {
	rows, err := p.pool.Query(ctx, `SELECT b.category, b.amount, COALESCE(GREATEST(sum(s.amount), 0), 0)
		FROM budgets b LEFT JOIN spending s ON s.account_id = b.account_id
			AND (b.category = 'TOTAL' OR s.category = b.category) AND s.occurred_at >= $2 AND s.occurred_at < $3
		WHERE b.account_id = $1 GROUP BY b.category, b.amount
		ORDER BY b.category <> 'TOTAL', b.amount DESC, b.category`, account, start, start.AddDate(0, 1, 0))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Budget, error) {
		var b Budget
		return b, r.Scan(&b.Category, &b.Amount, &b.Spent)
	})
}

// checkBudgets runs after money was spent (in the same transaction): for
// the category's budget and the TOTAL one, it sends each alert the month's
// spending reached and that was not sent yet. Only the current month alerts:
// an old event arriving late tells nobody about a month that is over.
// Locking the budget row serializes two expenses of the same account, so
// the second sees the first and an alert is never missed.
func (p *Projector) checkBudgets(ctx context.Context, tx pgx.Tx, account, category string, at time.Time) error {
	start, _ := ParseMonth(at.In(Brasilia).Format("2006-01"), at)
	if current, _ := ParseMonth("", p.now()); !current.Equal(start) {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT category, amount FROM budgets WHERE account_id = $1 AND category IN ($2, 'TOTAL')
		ORDER BY category FOR UPDATE`, account, category)
	if err != nil {
		return err
	}
	budgets, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Budget, error) {
		var b Budget
		return b, r.Scan(&b.Category, &b.Amount)
	})
	if err != nil {
		return err
	}
	month := start.Format("2006-01")
	for _, b := range budgets {
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(amount), 0) FROM spending WHERE account_id = $1
			AND ($2 = 'TOTAL' OR category = $2) AND occurred_at >= $3 AND occurred_at < $4`,
			account, b.Category, start, start.AddDate(0, 1, 0)).Scan(&b.Spent); err != nil {
			return err
		}
		for _, th := range Thresholds {
			if b.Spent*100 < b.Amount*int64(th) {
				continue
			}
			tag, err := tx.Exec(ctx, `INSERT INTO budget_alerts (account_id, category, month, threshold, spent)
				VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`, account, b.Category, month, th, b.Spent)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 1 {
				if err := publishAlert(ctx, tx, account, b, month, th); err != nil {
					return err
				}
			}
			break // 100% reached: the 80% alert would say less, later
		}
	}
	return nil
}

func publishAlert(ctx context.Context, tx pgx.Tx, account string, b Budget, month string, threshold int) error {
	ev, err := outbox.NewEvent(ctx, "statement-service", "statement.budget.threshold", map[string]any{
		"account_id": account, "category": b.Category, "month": month, "threshold": threshold, "budget": b.Amount, "spent": b.Spent})
	if err != nil {
		return err
	}
	return outbox.Insert(ctx, tx, outbox.Message{Topic: "bank.statement.events", Key: account, Event: ev})
}
