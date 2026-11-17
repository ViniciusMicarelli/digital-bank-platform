// Package inbox makes event consumers idempotent: an event's side effects and
// the record that it was processed are committed in the same transaction.
//
// Every consuming service includes this table in its migrations:
//
//	CREATE TABLE inbox (
//	    consumer     TEXT        NOT NULL,
//	    event_id     TEXT        NOT NULL,
//	    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
//	    PRIMARY KEY (consumer, event_id)
//	);
package inbox

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
)

// Handle runs fn for ev at most once per consumer. Duplicate deliveries
// (Kafka is at-least-once) are skipped. If fn fails nothing is recorded, so
// the event is retried.
func Handle(ctx context.Context, pool *pgxpool.Pool, consumer string, ev outbox.Event, fn func(pgx.Tx) error) error {
	return postgres.InTx(ctx, pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`INSERT INTO inbox (consumer, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, consumer, ev.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // already processed
		}
		return fn(tx)
	})
}
