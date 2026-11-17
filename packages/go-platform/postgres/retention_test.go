package postgres_test

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
)

func TestRetention(t *testing.T) {
	fs := fstest.MapFS{"0001_t.up.sql": {Data: []byte(`
		CREATE TABLE outbox (id INT, published_at TIMESTAMPTZ);
		INSERT INTO outbox SELECT g, now() - interval '10 days' FROM generate_series(1, 2500) g;
		INSERT INTO outbox SELECT g, now() FROM generate_series(1, 5) g;
		INSERT INTO outbox SELECT g, NULL FROM generate_series(1, 3) g;`)}}
	pool := pgtest.NewDB(t, fs, ".")
	ctx := context.Background()
	n, err := postgres.ApplyRetention(ctx, pool, []postgres.RetentionRule{postgres.OutboxRetention(7 * 24 * time.Hour)})
	if err != nil || n["outbox"] != 2500 {
		t.Fatalf("deleted = %v, %v", n, err)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&left)
	if left != 8 {
		t.Fatalf("left %d rows: recent and unpublished rows must stay", left)
	}
}
