// Package outbox implements the transactional outbox (ADR-0007): events are
// written to an "outbox" table in the same transaction as the state change,
// and a relay publishes them to Kafka afterwards (at-least-once).
//
// Every service that publishes events includes this table in its migrations:
//
//	CREATE TABLE outbox (
//	    id           UUID PRIMARY KEY,
//	    topic        TEXT NOT NULL,
//	    key          TEXT NOT NULL,
//	    payload      JSONB NOT NULL,
//	    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
//	    published_at TIMESTAMPTZ
//	);
//	CREATE INDEX outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/correlation"
)

// Event is the envelope shared by all domain events (docs/events/README.md).
type Event struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Version       int             `json:"version"`
	Source        string          `json:"source"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

// NewEvent builds an envelope for payload, taking the correlation ID from ctx.
func NewEvent(ctx context.Context, source, eventType string, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("outbox: marshal %s: %w", eventType, err)
	}
	return Event{
		ID:            uuid.Must(uuid.NewV7()).String(),
		Type:          eventType,
		Version:       1,
		Source:        source,
		OccurredAt:    time.Now().UTC(),
		CorrelationID: correlation.ID(ctx),
		Payload:       raw,
	}, nil
}

// Message is an event bound for a Kafka topic. Key sets the partition, so all
// events for the same aggregate keep their order.
type Message struct {
	Topic string
	Key   string
	Event Event
}

// Insert stores msg in the outbox inside tx.
func Insert(ctx context.Context, tx pgx.Tx, msg Message) error {
	body, err := json.Marshal(msg.Event)
	if err != nil {
		return fmt.Errorf("outbox: marshal envelope: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox (id, topic, key, payload) VALUES ($1, $2, $3, $4)`,
		msg.Event.ID, msg.Topic, msg.Key, body)
	if err != nil {
		return fmt.Errorf("outbox: insert: %w", err)
	}
	return nil
}
