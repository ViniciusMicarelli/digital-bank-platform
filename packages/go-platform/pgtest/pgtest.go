// Package pgtest creates an isolated, migrated Postgres database per test.
//
// It connects to TEST_DATABASE_URL (default: the docker-compose Postgres on
// localhost:5432). When Postgres is unreachable the test is skipped, unless
// PGTEST_REQUIRED=1 (set in CI), in which case it fails.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
)

const defaultURL = "postgres://bank:bank@localhost:54329/postgres?sslmode=disable"

// NewDB creates a fresh database, applies the migrations in dir of fsys and
// returns a pool to it. The database is dropped when the test ends.
func NewDB(t testing.TB, fsys fs.FS, dir string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		adminURL = defaultURL
	}
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		if os.Getenv("PGTEST_REQUIRED") == "1" {
			t.Fatalf("pgtest: postgres unavailable: %v", err)
		}
		t.Skipf("pgtest: postgres unavailable (run `make up`): %v", err)
	}
	defer admin.Close(ctx)

	name := "test_" + randomSuffix()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("pgtest: create database: %v", err)
	}

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("pgtest: parse url: %v", err)
	}
	u.Path = "/" + name
	pool, err := postgres.Connect(ctx, u.String())
	if err != nil {
		t.Fatalf("pgtest: connect: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if conn, err := pgx.Connect(ctx, adminURL); err == nil {
			_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			_ = conn.Close(ctx)
		}
	})
	if err := postgres.Migrate(ctx, pool, fsys, dir); err != nil {
		t.Fatalf("pgtest: migrate: %v", err)
	}
	return pool
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
