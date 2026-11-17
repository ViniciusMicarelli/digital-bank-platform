package application_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/migrations"
)

type fakeAccounts map[uuid.UUID]string

func (f fakeAccounts) Customer(_ context.Context, id uuid.UUID) (string, error) {
	c, ok := f[id]
	if !ok {
		return "", domain.ErrNotFound
	}
	return c, nil
}

type fakeSender struct {
	mu   sync.Mutex
	sent []domain.Channel
}

func (f *fakeSender) Send(_ context.Context, ch domain.Channel, _ domain.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, ch)
	return nil
}

func event(t *testing.T, typ string, p any) outbox.Event {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return outbox.Event{ID: uuid.NewString(), Type: typ, OccurredAt: time.Now(), Payload: raw}
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestNotifications(t *testing.T) {
	ctx := context.Background()
	store := pgstore.NewStore(pgtest.NewDB(t, migrations.FS, "."))
	acc, other := uuid.New(), uuid.New()
	sender := &fakeSender{}
	svc := application.New(store, fakeAccounts{other: "cust-2"}, sender, quiet)

	if err := svc.HandleEvent(ctx, event(t, "account.opened", map[string]any{"account_id": acc, "customer_id": "cust-1"})); err != nil {
		t.Fatal(err)
	}
	pix := event(t, "pix.transfer.received", map[string]any{"account_id": acc, "amount_minor": 5000, "counterparty_name": "Bruno"})
	for range 2 { // redelivered by Kafka
		if err := svc.HandleEvent(ctx, pix); err != nil {
			t.Fatal(err)
		}
	}
	list, unread, err := svc.List(ctx, "cust-1", false, uuid.Nil, 10)
	if err != nil || len(list) != 1 || unread != 1 || list[0].Body != "Você recebeu R$ 50,00 de Bruno." || len(list[0].Channels) != 2 {
		t.Fatalf("list = %+v unread %d %v", list, unread, err)
	}
	if svc.DeliverOnce(ctx); len(sender.sent) != 1 || sender.sent[0] != domain.Push {
		t.Fatalf("deliveries = %v", sender.sent)
	}
	if svc.DeliverOnce(ctx); len(sender.sent) != 1 {
		t.Fatal("a delivery is sent once")
	}

	// The account's owner is asked to account-service when unknown.
	_ = svc.SetPreferences(ctx, "cust-2", domain.Preferences{Push: false, Email: true})
	if err := svc.HandleEvent(ctx, event(t, "account.blocked", map[string]any{"account_id": other})); err != nil {
		t.Fatal(err)
	}
	list, _, _ = svc.List(ctx, "cust-2", true, uuid.Nil, 10)
	if len(list) != 1 || list[0].Channels[1] != domain.Email {
		t.Fatalf("other customer = %+v", list)
	}
	if err := svc.HandleEvent(ctx, event(t, "account.blocked", map[string]any{"account_id": uuid.New()})); err != nil {
		t.Fatal("an unknown account is skipped, not retried forever")
	}

	n, _ := svc.MarkRead(ctx, "cust-2", []uuid.UUID{list[0].ID})
	if _, unread, _ := svc.List(ctx, "cust-2", false, uuid.Nil, 10); n != 1 || unread != 0 {
		t.Fatalf("mark read: %d, unread %d", n, unread)
	}
	if n, _ := svc.MarkRead(ctx, "cust-1", nil); n != 1 {
		t.Fatalf("mark all read: %d", n)
	}
}

// A notification created by one replica reaches a client streaming from
// another, through LISTEN / NOTIFY (polling is set too slow to matter).
func TestStreamAcrossReplicas(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	acc := uuid.New()
	writer := application.New(pgstore.NewStore(pool), fakeAccounts{acc: "cust-1"}, &fakeSender{}, quiet)
	readerStore := pgstore.NewStore(pool)
	reader := application.New(readerStore, fakeAccounts{acc: "cust-1"}, &fakeSender{}, quiet)
	go readerStore.Listen(ctx, quiet, reader.Wake)

	// Something from before the stream opened is not replayed.
	_ = writer.HandleEvent(ctx, event(t, "pix.transfer.received", map[string]any{"account_id": acc, "amount_minor": 100, "counterparty_name": "Old"}))

	got := make(chan domain.Notification, 1)
	go func() {
		_ = reader.Stream(ctx, "cust-1", uuid.Nil, time.Hour, func(n domain.Notification) error {
			got <- n
			return nil
		})
	}()
	time.Sleep(300 * time.Millisecond) // listener and stream ready
	start := time.Now()
	_ = writer.HandleEvent(ctx, event(t, "pix.transfer.received", map[string]any{"account_id": acc, "amount_minor": 990, "counterparty_name": "Carla"}))
	select {
	case n := <-got:
		if n.Body != "Você recebeu R$ 9,90 de Carla." {
			t.Fatalf("streamed %+v", n)
		}
		t.Logf("streamed across replicas in %v", time.Since(start))
	case <-ctx.Done():
		t.Fatal("the notification never reached the stream")
	}
}
