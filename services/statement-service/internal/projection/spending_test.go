package projection_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/internal/projection"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/migrations"
)

func at(ev outbox.Event, t time.Time) outbox.Event {
	ev.OccurredAt = t
	return ev
}

func TestCategory(t *testing.T) {
	for mcc, want := range map[string]string{"5462": projection.Food, "4899": projection.Subscriptions, "3005": projection.Travel,
		"5661": projection.Shopping, "8050": projection.Health, "1234": projection.Other, "x": projection.Other} {
		if got := projection.Category(mcc); got != want {
			t.Errorf("Category(%s) = %s, want %s", mcc, got, want)
		}
	}
}

func TestSpending(t *testing.T) {
	p := projection.New(pgtest.NewDB(t, migrations.FS, "."))
	ctx := context.Background()
	ana, card, other := uuid.New(), uuid.New(), uuid.New()
	nov := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	oct := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	cleared := func(auth string, amount int64, merchant, mcc string, when time.Time, withAccount bool) outbox.Event {
		payload := map[string]any{"authorization_id": auth, "card_id": card.String(), "cleared_minor": amount, "merchant_name": merchant, "mcc": mcc,
			"account_id": uuid.Nil.String()}
		if withAccount {
			payload["account_id"] = ana.String()
		}
		return at(event(t, "card.purchase.cleared", payload), when)
	}
	shoes, tv := uuid.NewString(), uuid.NewString()
	for _, ev := range []outbox.Event{
		// A refund that arrives before its purchase (another partition).
		at(event(t, "card.purchase.refunded", map[string]any{"refund_id": uuid.NewString(), "authorization_id": tv, "amount_minor": 40_00}), nov.Add(3*time.Hour)),
		cleared(tv, 150_00, "LOJA DE TV", "5732", nov, true),
		event(t, "card.issued", map[string]any{"card_id": card.String(), "account_id": ana.String()}),
		// An old event without the account: resolved through the card.
		cleared(uuid.NewString(), 40_00, "PADARIA", "5462", nov, false),
		cleared(uuid.NewString(), 60_00, "PADARIA", "5462", nov.Add(time.Hour), true),
		cleared(shoes, 300_00, "LOJA DE CALCADOS", "5661", nov, true),
		at(event(t, "card.purchase.refunded", map[string]any{"refund_id": uuid.NewString(), "authorization_id": shoes, "amount_minor": 100_00}), nov.Add(2*time.Hour)),
		cleared(uuid.NewString(), 500_00, "PASSAGENS", "3005", oct, true),
		completed(t, "PIX_OUT", ana, other, 80_00, nov, map[string]string{"counterparty_name": "Maria"}),
		completed(t, "BOLETO_PAYMENT", ana, other, 120_00, nov, map[string]string{"counterparty_name": "CIA DE ENERGIA"}),
		completed(t, "CREDIT_INVOICE_PAYMENT", ana, other, 999_00, nov, nil), // not spending
		completed(t, "SAVINGS_DEPOSIT", ana, other, 999_00, nov, nil),        // not spending
	} {
		for range 2 { // redelivery
			if err := p.Handle(ctx, ev); err != nil {
				t.Fatal(err)
			}
		}
	}
	start, _ := projection.ParseMonth("2026-11", time.Now())
	s, err := p.Spending(ctx, ana, start)
	if err != nil {
		t.Fatal(err)
	}
	// 100 food + 310 shopping (300 - 100 + 150 - 40 refunds) + 80 Pix + 120 bills.
	if s.Total != 610_00 || s.PreviousTotal != 500_00 || len(s.Categories) != 4 {
		t.Fatalf("spending = %+v", s)
	}
	if c := s.Categories[0]; c.Category != projection.Shopping || c.Amount != 310_00 || c.Count != 2 {
		t.Fatalf("largest = %+v", c)
	}
	if len(s.Merchants) != 4 || s.Merchants[0].Name != "LOJA DE CALCADOS" || s.Merchants[3].Name != "PADARIA" || s.Merchants[3].Count != 2 {
		t.Fatalf("merchants = %+v", s.Merchants)
	}
	if _, err := projection.ParseMonth("11/2026", time.Now()); err == nil {
		t.Fatal("bad month")
	}
}
