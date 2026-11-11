# ADR-0007: Transactional Outbox for Event Publishing

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Writing to the database and publishing to Kafka as two separate operations (dual write) can leave the system inconsistent: the DB commits but the event is lost, or the event is published but the DB rolls back.

## Decision

Services write events to an `outbox` table **in the same transaction** as the state change. A relay process from `packages/go-messaging` publishes pending rows to Kafka and marks them as published.

## Alternatives Considered

- **Dual write with retries**: still inconsistent on crashes.
- **CDC (Debezium) only**: robust, but adds another component. It remains an option for the relay in the future.

## Consequences

- ✅ Atomicity between state and events.
- ✅ At-least-once delivery.
- ⚠️ Consumers must deduplicate (inbox).
- ⚠️ Small publishing latency (polling interval) and outbox table maintenance (cleanup job).
