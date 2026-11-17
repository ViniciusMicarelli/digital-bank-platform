// Package postgres holds the Postgres plumbing shared by services:
// connection pools, transactions with retry, and embedded SQL migrations.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pool and verifies the connection.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("postgres: open pool: %w", err)
	}
	// Wait up to 30 s for the database (starting up, failing over).
	deadline := time.Now().Add(30 * time.Second)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			pool.Close()
			return nil, fmt.Errorf("postgres: ping: %w", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

// SQLSTATE codes that are safe to retry: the whole transaction is replayed.
const (
	codeSerializationFailure = "40001"
	codeDeadlockDetected     = "40P01"
	codeUniqueViolation      = "23505"
)

// InTx runs fn in a transaction and commits it, retrying a few times on
// serialization failures and deadlocks. fn must be safe to re-run.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	const attempts = 5
	var err error
	for i := range attempts {
		err = pgx.BeginFunc(ctx, pool, fn)
		if !isRetryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * 10 * time.Millisecond):
		}
	}
	return err
}

func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		(pgErr.Code == codeSerializationFailure || pgErr.Code == codeDeadlockDetected)
}

// IsUniqueViolation reports whether err is a unique constraint violation,
// optionally on a specific constraint.
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeUniqueViolation &&
		(constraint == "" || pgErr.ConstraintName == constraint)
}

// Migrate applies every "*.up.sql" file in dir of fsys that has not been
// applied yet, in lexical order, each in its own transaction. Versions are the
// file name prefix before the first "_" (e.g. "0001_init.up.sql" -> "0001").
// A Postgres advisory lock makes concurrent runs (several replicas booting) safe.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("migrate: read %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	slices.Sort(files)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquire: %w", err)
	}
	defer conn.Release()

	const lockID = 7_365_221 // arbitrary, shared by all services' migrators
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		return fmt.Errorf("migrate: lock: %w", err)
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, lockID) //nolint:errcheck

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("migrate: create table: %w", err)
	}

	for _, name := range files {
		version, _, _ := strings.Cut(name, "_")
		var exists bool
		if err := conn.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("migrate: check %s: %w", name, err)
		}
		if exists {
			continue
		}
		sql, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", name, err)
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return fmt.Errorf("migrate: apply %s: %w", name, err)
		}
	}
	return nil
}
