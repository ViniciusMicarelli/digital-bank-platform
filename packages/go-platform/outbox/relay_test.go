package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/correlation"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/inbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
)

var schema = fstest.MapFS{
	"0001_outbox.up.sql": {Data: []byte(`
		CREATE TABLE outbox (id UUID PRIMARY KEY, topic TEXT NOT NULL, key TEXT NOT NULL, payload JSONB NOT NULL,
		                     created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ);
		CREATE TABLE inbox (consumer TEXT NOT NULL, event_id TEXT NOT NULL,
		                    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (consumer, event_id));`)},
	"0002_counter.up.sql": {Data: []byte(`CREATE TABLE counter (n INT NOT NULL); INSERT INTO counter VALUES (0);`)},
}

type fakePublisher struct {
	mu   sync.Mutex
	got  []outbox.Record
	fail bool
}

func (p *fakePublisher) Publish(_ context.Context, rs []outbox.Record) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail {
		return errors.New("broker down")
	}
	p.got = append(p.got, rs...)
	return nil
}

func TestRelayPublishesInOrderOnce(t *testing.T) {
	pool := pgtest.NewDB(t, schema, ".")
	ctx := correlation.WithID(context.Background(), "corr-1")
	for i := range 3 {
		ev, err := outbox.NewEvent(ctx, "test", "thing.happened", map[string]int{"i": i})
		if err != nil {
			t.Fatal(err)
		}
		err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			return outbox.Insert(ctx, tx, outbox.Message{Topic: "bank.test.events", Key: "k", Event: ev})
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	pub := &fakePublisher{fail: true}
	relay := outbox.NewRelay(pool, pub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := relay.PublishBatch(ctx); err == nil {
		t.Fatal("expected publish error")
	}
	var pending int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&pending)
	if pending != 3 {
		t.Fatalf("failed publish must keep rows pending, got %d", pending)
	}

	pub.fail = false
	if n, err := relay.PublishBatch(ctx); err != nil || n != 3 {
		t.Fatalf("publish = %d, %v", n, err)
	}
	if n, _ := relay.PublishBatch(ctx); n != 0 {
		t.Fatalf("rows published twice: %d", n)
	}
	if len(pub.got) != 3 || pub.got[0].Headers["event_type"] != "thing.happened" || pub.got[0].Headers["correlation_id"] != "corr-1" {
		t.Fatalf("records = %+v", pub.got)
	}
	for i, r := range pub.got {
		var want = []string{`"i": 0`, `"i": 1`, `"i": 2`}[i]
		if !contains(string(r.Value), want) {
			t.Errorf("record %d out of order: %s", i, r.Value)
		}
	}
}

func TestInboxProcessesOnce(t *testing.T) {
	pool := pgtest.NewDB(t, schema, ".")
	ctx := context.Background()
	ev, _ := outbox.NewEvent(ctx, "test", "thing.happened", nil)
	incr := func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE counter SET n = n + 1`)
		return err
	}
	for range 3 {
		if err := inbox.Handle(ctx, pool, "c1", ev, incr); err != nil {
			t.Fatal(err)
		}
	}
	// A failing handler records nothing, so the event can be retried.
	ev2, _ := outbox.NewEvent(ctx, "test", "thing.happened", nil)
	if err := inbox.Handle(ctx, pool, "c1", ev2, func(pgx.Tx) error { return errors.New("boom") }); err == nil {
		t.Fatal("expected error")
	}
	if err := inbox.Handle(ctx, pool, "c1", ev2, incr); err != nil {
		t.Fatal(err)
	}
	// Another consumer of the same event has its own record.
	if err := inbox.Handle(ctx, pool, "c2", ev, incr); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT n FROM counter`).Scan(&n)
	if n != 3 {
		t.Fatalf("counter = %d, want 3", n)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
