// Package kafka wraps franz-go: a producer for the outbox relay and a
// consumer-group runner with retries, a dead-letter topic and manual commits.
package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/correlation"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
)

// Brokers splits a comma-separated broker list.
func Brokers(s string) []string {
	var out []string
	for _, b := range strings.Split(s, ",") {
		if b = strings.TrimSpace(b); b != "" {
			out = append(out, b)
		}
	}
	return out
}

// Producer implements outbox.Publisher with idempotent, acks=all writes.
type Producer struct{ client *kgo.Client }

func NewProducer(brokers []string) (*Producer, error) {
	c, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.AllowAutoTopicCreation(),
		kgo.ProducerLinger(5*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka: producer: %w", err)
	}
	return &Producer{client: c}, nil
}

func (p *Producer) Close() { p.client.Close() }

// Publish produces all records and waits for every acknowledgement.
func (p *Producer) Publish(ctx context.Context, records []outbox.Record) error {
	rs := make([]*kgo.Record, len(records))
	for i, r := range records {
		rec := &kgo.Record{Topic: r.Topic, Key: []byte(r.Key), Value: r.Value}
		for k, v := range r.Headers {
			rec.Headers = append(rec.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
		}
		rs[i] = rec
	}
	return p.client.ProduceSync(ctx, rs...).FirstErr()
}

// Handler processes one event. Returning an error retries it; after
// MaxAttempts it goes to the dead-letter topic "<topic>.dlq".
type Handler func(ctx context.Context, ev outbox.Event) error

// ConsumerConfig configures Consume.
type ConsumerConfig struct {
	Brokers     []string
	Group       string
	Topics      []string
	MaxAttempts int // default 5
}

// Consume runs a consumer group until ctx is cancelled. Offsets are committed
// only after the handler succeeded (or the record was dead-lettered), so
// delivery is at-least-once: handlers must be idempotent (see inbox).
func Consume(ctx context.Context, cfg ConsumerConfig, log *slog.Logger, handle Handler) error {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.Group),
		kgo.ConsumeTopics(cfg.Topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
		kgo.AllowAutoTopicCreation(),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		return fmt.Errorf("kafka: consumer: %w", err)
	}
	defer client.Close()

	for {
		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			return nil
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			log.WarnContext(ctx, "kafka fetch", "topic", topic, "partition", partition, "error", err)
		})
		var processed []*kgo.Record
		fetches.EachRecord(func(rec *kgo.Record) {
			if ctx.Err() != nil {
				return
			}
			if err := process(ctx, client, cfg, log, handle, rec); err != nil {
				return // ctx cancelled mid-retry: do not commit
			}
			processed = append(processed, rec)
		})
		if len(processed) > 0 {
			if err := client.CommitRecords(context.WithoutCancel(ctx), processed...); err != nil {
				log.WarnContext(ctx, "kafka commit", "error", err)
			}
		}
	}
}

func process(ctx context.Context, client *kgo.Client, cfg ConsumerConfig, log *slog.Logger, handle Handler, rec *kgo.Record) error {
	var ev outbox.Event
	if err := json.Unmarshal(rec.Value, &ev); err != nil {
		log.ErrorContext(ctx, "undecodable event, dead-lettering", "topic", rec.Topic, "error", err)
		return deadLetter(ctx, client, rec, err)
	}
	evCtx := correlation.WithID(ctx, ev.CorrelationID)
	var err error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err = handle(evCtx, ev); err == nil {
			return nil
		}
		log.WarnContext(evCtx, "event handler failed", "event_type", ev.Type, "event_id", ev.ID, "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt*attempt) * 100 * time.Millisecond):
		}
	}
	log.ErrorContext(evCtx, "event dead-lettered", "event_type", ev.Type, "event_id", ev.ID, "error", err)
	return deadLetter(ctx, client, rec, err)
}

func deadLetter(ctx context.Context, client *kgo.Client, rec *kgo.Record, cause error) error {
	dlq := &kgo.Record{
		Topic: rec.Topic + ".dlq", Key: rec.Key, Value: rec.Value,
		Headers: append(rec.Headers, kgo.RecordHeader{Key: "error", Value: []byte(cause.Error())}),
	}
	if err := client.ProduceSync(ctx, dlq).FirstErr(); err != nil {
		return errors.Join(cause, err)
	}
	return nil
}

// RunOutboxRelay publishes the service's outbox to Kafka until ctx ends. With
// no brokers configured it logs a warning and returns: events stay in the
// outbox and are published once Kafka is configured.
func RunOutboxRelay(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, brokers []string) {
	if len(brokers) == 0 {
		log.WarnContext(ctx, "KAFKA_BROKERS not set: outbox relay disabled")
		return
	}
	prod, err := NewProducer(brokers)
	if err != nil {
		log.ErrorContext(ctx, "outbox relay disabled", "error", err)
		return
	}
	defer prod.Close()
	log.InfoContext(ctx, "outbox relay started", "brokers", brokers)
	outbox.NewRelay(pool, prod, log).Run(ctx, 200*time.Millisecond)
}
