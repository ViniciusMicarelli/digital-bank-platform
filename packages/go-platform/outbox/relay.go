package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
)

// Record is a message ready for the broker.
type Record struct {
	Topic   string
	Key     string
	Value   []byte
	Headers map[string]string
}

// Publisher sends records and returns only once the broker acknowledged all
// of them (or an error).
type Publisher interface {
	Publish(ctx context.Context, records []Record) error
}

// Relay moves outbox rows to the broker.
type Relay struct {
	pool      *pgxpool.Pool
	pub       Publisher
	log       *slog.Logger
	batchSize int
}

func NewRelay(pool *pgxpool.Pool, pub Publisher, log *slog.Logger) *Relay {
	return &Relay{pool: pool, pub: pub, log: log, batchSize: 200}
}

// PublishBatch publishes the oldest unpublished rows and marks them as
// published, in one transaction. Rows are locked with SKIP LOCKED, so several
// relay replicas can run side by side. If publishing fails the transaction
// rolls back and the rows are retried later: delivery is at-least-once, and
// consumers deduplicate by event id (see inbox). With one relay per service,
// events keep their creation order; with several, order holds per batch only.
func (r *Relay) PublishBatch(ctx context.Context) (int, error) {
	n := 0
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, topic, key, payload FROM outbox
			 WHERE published_at IS NULL
			 ORDER BY created_at, id
			 LIMIT $1
			   FOR UPDATE SKIP LOCKED`, r.batchSize)
		if err != nil {
			return err
		}
		var (
			ids     []string
			records []Record
		)
		for rows.Next() {
			var (
				id, topic, key string
				payload        []byte
			)
			if err := rows.Scan(&id, &topic, &key, &payload); err != nil {
				return err
			}
			var ev Event
			if err := json.Unmarshal(payload, &ev); err != nil {
				return fmt.Errorf("outbox: decode %s: %w", id, err)
			}
			ids = append(ids, id)
			records = append(records, Record{
				Topic: topic, Key: key, Value: payload,
				Headers: map[string]string{"event_type": ev.Type, "event_id": ev.ID, "correlation_id": ev.CorrelationID},
			})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		// Bound the broker call: the row locks are held until it returns.
		pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := r.pub.Publish(pubCtx, records); err != nil {
			return fmt.Errorf("outbox: publish: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = ANY($1)`, ids); err != nil {
			return err
		}
		n = len(ids)
		telemetry.OutboxPublished.Add(float64(n))
		return nil
	})
	return n, err
}

// Run publishes continuously: immediately again while there is a backlog,
// otherwise every interval. Errors are logged and retried with backoff.
func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	backoff := interval
	for {
		n, err := r.PublishBatch(ctx)
		wait := interval
		switch {
		case err != nil && ctx.Err() == nil:
			r.log.WarnContext(ctx, "outbox relay", "error", err)
			telemetry.OutboxErrors.Inc()
			backoff = min(backoff*2, 30*time.Second)
			wait = backoff
		case n > 0:
			backoff, wait = interval, 0
		default:
			backoff = interval
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
