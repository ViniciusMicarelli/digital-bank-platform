package projection_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/internal/projection"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/migrations"
)

func TestBudgets(t *testing.T) {
	pool := pgtest.NewDB(t, migrations.FS, ".")
	p := projection.New(pool)
	now := time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)
	p.SetClock(func() time.Time { return now })
	ctx := context.Background()
	ana, other := uuid.New(), uuid.New()
	nov, _ := projection.ParseMonth("2026-11", now)

	if err := p.SetBudget(ctx, ana, "PETS", 100_00); !errors.Is(err, projection.ErrInvalidBudget) {
		t.Fatalf("unknown category: %v", err)
	}
	_ = p.SetBudget(ctx, ana, projection.Transfers, 100_00)
	_ = p.SetBudget(ctx, ana, projection.Total, 1_000_00)
	_ = p.SetBudget(ctx, ana, projection.Food, 50_00)
	_ = p.SetBudget(ctx, ana, projection.Food, 0) // removed

	alerts := func() (list []string) {
		rows, _ := pool.Query(ctx, `SELECT payload->'payload'->>'category' || ':' || (payload->'payload'->>'threshold')
			FROM outbox ORDER BY created_at, payload->'payload'->>'threshold'`)
		defer rows.Close()
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			list = append(list, s)
		}
		return list
	}
	pix := func(amount int64, when time.Time) {
		t.Helper()
		if err := p.Handle(ctx, completed(t, "PIX_OUT", ana, other, amount, when, map[string]string{"counterparty_name": "Maria"})); err != nil {
			t.Fatal(err)
		}
	}

	pix(70_00, now) // 70%: nothing yet
	if got := alerts(); len(got) != 0 {
		t.Fatalf("under 80%%: %v", got)
	}
	pix(15_00, now) // 85%
	pix(5_00, now)  // 90%: the 80% alert was sent already
	if got := alerts(); len(got) != 1 || got[0] != "TRANSFERS:80" {
		t.Fatalf("at 80%%: %v", got)
	}
	pix(950_00, now) // transfers past 100%, the total too: straight to 100%, no 80% alert
	if got := alerts(); len(got) != 3 || got[1] != "TOTAL:100" || got[2] != "TRANSFERS:100" {
		t.Fatalf("past 100%%: %v", got)
	}
	// A late event of an earlier month alerts nobody.
	_ = p.SetBudget(ctx, ana, projection.Bills, 10_00)
	if err := p.Handle(ctx, completed(t, "BOLETO_PAYMENT", ana, other, 50_00, now.AddDate(0, -1, 0), nil)); err != nil {
		t.Fatal(err)
	}
	if got := alerts(); len(got) != 3 {
		t.Fatalf("an old month alerted: %v", got)
	}

	list, err := p.Budgets(ctx, ana, nov)
	if err != nil || len(list) != 3 || list[0].Category != projection.Total || list[0].Spent != 1_040_00 {
		t.Fatalf("budgets = %+v %v", list, err)
	}
	for _, b := range list {
		if b.Category == projection.Bills && b.Spent != 0 {
			t.Fatalf("bills this month = %+v", b)
		}
	}
}
