package postgres_test

import (
	"context"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
)

func TestMigrateIsIdempotentAndConcurrent(t *testing.T) {
	fs := fstest.MapFS{
		"0001_a.up.sql": {Data: []byte(`CREATE TABLE a (id INT)`)},
		"0002_b.up.sql": {Data: []byte(`CREATE TABLE b (id INT)`)},
		"README.md":     {Data: []byte(`ignored`)},
	}
	pool := pgtest.NewDB(t, fs, ".")
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			if err := postgres.Migrate(ctx, pool, fs, "."); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	fs["0003_c.up.sql"] = &fstest.MapFile{Data: []byte(`CREATE TABLE c (id INT)`)}
	if err := postgres.Migrate(ctx, pool, fs, "."); err != nil {
		t.Fatal(err)
	}
	var versions int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&versions)
	if versions != 3 {
		t.Fatalf("versions = %d", versions)
	}

	bad := fstest.MapFS{"0004_bad.up.sql": {Data: []byte(`CREATE TABLE d (; nonsense`)}}
	if err := postgres.Migrate(ctx, pool, bad, "."); err == nil {
		t.Fatal("expected failure")
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&versions)
	if versions != 3 {
		t.Fatal("a failed migration must not be recorded")
	}
}
