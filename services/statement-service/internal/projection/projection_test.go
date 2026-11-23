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

func event(t *testing.T, typ string, payload any) outbox.Event {
	t.Helper()
	ev, err := outbox.NewEvent(context.Background(), "test", typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func opened(t *testing.T, id uuid.UUID, name string) outbox.Event {
	return event(t, "account.opened", map[string]string{
		"account_id": id.String(), "holder_name": name, "branch": "0001", "number": "00001001-7", "status": "ACTIVE",
	})
}

func completed(t *testing.T, typ string, from, to uuid.UUID, amount int64, at time.Time, md map[string]string) outbox.Event {
	return event(t, "transaction.completed", map[string]any{
		"transaction_id": uuid.NewString(), "type": typ, "status": "COMPLETED",
		"from_account_id": from.String(), "to_account_id": to.String(),
		"amount_minor": amount, "currency": "BRL", "description": "d", "metadata": md, "occurred_at": at,
	})
}

func TestFeedProjection(t *testing.T) {
	p := projection.New(pgtest.NewDB(t, migrations.FS, "."))
	ctx := context.Background()
	ana, bruno, settlement := uuid.New(), uuid.New(), uuid.New()
	t0 := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	// The transfer arrives before Bruno's account.opened: the name is resolved at read time.
	transfer := completed(t, "INTERNAL_TRANSFER", ana, bruno, 30_00, t0, nil)
	for _, ev := range []outbox.Event{
		transfer, transfer, // redelivery
		opened(t, ana, "Ana Silva"), opened(t, bruno, "Bruno Lima"),
		completed(t, "PIX_OUT", ana, settlement, 10_00, t0.Add(time.Minute), map[string]string{"counterparty_name": "Maria Oliveira", "end_to_end_id": "E1"}),
		completed(t, "PIX_IN", settlement, ana, 5_00, t0.Add(2*time.Minute), map[string]string{"counterparty_name": "João Pereira", "end_to_end_id": "E2"}),
		event(t, "transaction.failed", map[string]string{"transaction_id": uuid.NewString()}),
		event(t, "something.new", map[string]string{}),
	} {
		if err := p.Handle(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}

	items, next, err := p.Feed(ctx, ana, "", 10)
	if err != nil || next != "" {
		t.Fatalf("feed = %v, next=%q, %v", items, next, err)
	}
	want := []struct{ typ, dir, who string }{
		{"PIX_IN", "CREDIT", "João Pereira"},
		{"PIX_OUT", "DEBIT", "Maria Oliveira"},
		{"INTERNAL_TRANSFER", "DEBIT", "Bruno Lima"},
	}
	if len(items) != len(want) {
		t.Fatalf("ana has %d items, want %d: %+v", len(items), len(want), items)
	}
	for i, w := range want {
		if items[i].Type != w.typ || items[i].Direction != w.dir || items[i].CounterpartyName != w.who {
			t.Errorf("item %d = %s %s %s, want %+v", i, items[i].Type, items[i].Direction, items[i].CounterpartyName, w)
		}
	}

	brunoItems, _, _ := p.Feed(ctx, bruno, "", 10)
	if len(brunoItems) != 1 || brunoItems[0].Direction != "CREDIT" || brunoItems[0].CounterpartyName != "Ana Silva" {
		t.Fatalf("bruno = %+v", brunoItems)
	}
	if s, _, _ := p.Feed(ctx, settlement, "", 10); len(s) != 0 {
		t.Fatal("internal accounts must not get feed items")
	}
}

func TestSameBankPixAndPagination(t *testing.T) {
	p := projection.New(pgtest.NewDB(t, migrations.FS, "."))
	ctx := context.Background()
	ana, bruno := uuid.New(), uuid.New()
	t0 := time.Now().UTC()
	for i := range 5 {
		ev := completed(t, "INTERNAL_TRANSFER", ana, bruno, int64(i+1)*100, t0.Add(time.Duration(i)*time.Second),
			map[string]string{"end_to_end_id": "E", "counterparty_name": "Bruno Lima"})
		if err := p.Handle(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	var seen []int64
	token := ""
	for {
		page, next, err := p.Feed(ctx, ana, token, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range page {
			if it.Type != "PIX" {
				t.Fatalf("same-bank Pix type = %s", it.Type)
			}
			seen = append(seen, it.AmountMinor)
		}
		if next == "" {
			break
		}
		token = next
	}
	if len(seen) != 5 || seen[0] != 500 || seen[4] != 100 {
		t.Fatalf("pages = %v", seen)
	}
	if _, _, err := p.Feed(ctx, ana, "garbage!", 2); err == nil {
		t.Fatal("expected invalid cursor error")
	}
}

func TestFeedFilters(t *testing.T) {
	p := projection.New(pgtest.NewDB(t, migrations.FS, "."))
	ctx := context.Background()
	ana, bruno, settlement := uuid.New(), uuid.New(), uuid.New()
	t0 := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	for _, ev := range []outbox.Event{
		opened(t, ana, "Ana Silva"), opened(t, bruno, "Bruno Lima"),
		completed(t, "INTERNAL_TRANSFER", ana, bruno, 30_00, t0, nil), // Bruno resolved from the accounts view
		completed(t, "PIX_OUT", ana, settlement, 10_00, t0.Add(time.Minute), map[string]string{"counterparty_name": "Maria Oliveira"}),
		completed(t, "PIX_IN", settlement, ana, 5_00, t0.Add(2*time.Minute), map[string]string{"counterparty_name": "Mariana 100% Sá"}),
		completed(t, "PIX_IN", settlement, ana, 7_00, t0.Add(3*time.Minute), map[string]string{"counterparty_name": "João Pereira"}),
	} {
		if err := p.Handle(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	names := func(f projection.FeedFilter) (out []string) {
		t.Helper()
		items, _, err := p.FeedFiltered(ctx, ana, f, "", 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			out = append(out, it.CounterpartyName)
		}
		return out
	}
	if got := names(projection.FeedFilter{Direction: "CREDIT"}); len(got) != 2 {
		t.Fatalf("money in = %v", got)
	}
	if got := names(projection.FeedFilter{Direction: "DEBIT"}); len(got) != 2 {
		t.Fatalf("money out = %v", got)
	}
	if got := names(projection.FeedFilter{Query: "maria"}); len(got) != 2 {
		t.Fatalf("maria = %v", got)
	}
	if got := names(projection.FeedFilter{Query: "bruno"}); len(got) != 1 || got[0] != "Bruno Lima" {
		t.Fatalf("resolved names are searched too: %v", got)
	}
	// % and _ are matched literally.
	if got := names(projection.FeedFilter{Query: "100%"}); len(got) != 1 || got[0] != "Mariana 100% Sá" {
		t.Fatalf("literal %%: %v", got)
	}
	if got := names(projection.FeedFilter{Query: "_"}); len(got) != 0 {
		t.Fatalf("literal _: %v", got)
	}
	if got := names(projection.FeedFilter{Query: "maria", Direction: "DEBIT"}); len(got) != 1 || got[0] != "Maria Oliveira" {
		t.Fatalf("both = %v", got)
	}
	// Pages of a filtered feed.
	page, next, _ := p.FeedFiltered(ctx, ana, projection.FeedFilter{Direction: "CREDIT"}, "", 1)
	rest, _, _ := p.FeedFiltered(ctx, ana, projection.FeedFilter{Direction: "CREDIT"}, next, 1)
	if len(page) != 1 || len(rest) != 1 || page[0].ID == rest[0].ID || rest[0].Direction != "CREDIT" {
		t.Fatalf("pages = %+v / %+v", page, rest)
	}
}
