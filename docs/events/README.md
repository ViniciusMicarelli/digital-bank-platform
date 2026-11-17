# Events

Services publish domain events to Kafka through the **transactional outbox** ([ADR-0004](../decisions/0004-kafka-for-events.md), [ADR-0007](../decisions/0007-transactional-outbox.md)). Consumers deduplicate with an **inbox** table.

## How Events Flow

```mermaid
sequenceDiagram
    participant S as Service
    participant DB as Service DB
    participant R as Outbox relay
    participant K as Kafka
    participant C as Consumer
    S->>DB: BEGIN; state change + INSERT outbox; COMMIT
    R->>DB: SELECT ... FOR UPDATE SKIP LOCKED
    R->>K: produce (acks=all), key = aggregate id
    R->>DB: UPDATE outbox SET published_at = now()
    K->>C: deliver (at-least-once)
    C->>C: BEGIN; INSERT inbox (event_id) ON CONFLICT DO NOTHING; handle; COMMIT; commit offset
```

- **Atomic.** An event exists if and only if its state change committed.
- **At-least-once.** If the relay crashes after producing but before marking rows, they are produced again. Consumers deduplicate by `id` in the inbox, inside the same transaction as their side effects (`packages/go-platform/inbox`).
- **Ordering.** The record key is the aggregate ID (entry, hold, account, transaction), so one aggregate's events land in one partition, in order.
- **Failures.** A consumer retries a failing event with backoff (5 attempts), then sends it to `<topic>.dlq` and moves on. Offsets are committed only after success or dead-lettering.

Code: `packages/go-platform/outbox` (envelope, insert, relay), `packages/go-platform/kafka` (producer, consumer group), `packages/go-platform/inbox`.

## Envelope

```json
{
  "id": "0193f1c2-7c1e-7a1b-9c55-0e7d3f1b2a10",
  "type": "transaction.completed",
  "version": 1,
  "source": "transaction-service",
  "occurred_at": "2026-11-17T16:30:23.421Z",
  "correlation_id": "e2e-6c1f...",
  "payload": { "...": "..." }
}
```

- `id`: UUIDv7, used for deduplication.
- `type`: `<domain>.<aggregate>.<past-tense verb>`.
- `version`: payload schema version. Changes are additive; a breaking change publishes a new version.
- `correlation_id`: the ID of the originating request, propagated through gRPC metadata (`x-correlation-id`).
- Amounts are always `*_minor` integers with a `currency`, never floats.

Kafka headers repeat `event_type`, `event_id` and `correlation_id` for routing and debugging without parsing the body.

## Topics and Catalog

| Topic | Event | Key | Payload highlights |
|---|---|---|---|
| `bank.ledger.events` | `ledger.entry.posted` | entry id | `entry_id`, `postings[]` (`account_id`, `amount_minor`, `currency`), `description`, `metadata`, `reverses_entry_id` |
| | `ledger.hold.created` / `ledger.hold.captured` / `ledger.hold.released` | hold id | `hold_id`, `account_id`, `counter_account_id`, `amount_minor`, `status`, `capture_entry_id` |
| `bank.account.events` | `account.opened` / `account.blocked` / `account.unblocked` / `account.closed` | account id | `account_id`, `customer_id`, `holder_name`, `branch`, `number`, `status`, `reason` |
| | `account.limits.changed` | account id | `per_transaction_minor`, `daily_minor`, `nightly_minor` |
| `bank.transaction.events` | `transaction.reserved` / `transaction.completed` / `transaction.failed` | transaction id | `transaction_id`, `type`, `from_account_id`, `to_account_id`, `amount_minor`, `description`, `metadata`, `failure_reason` |
| `bank.pix.events` | `pix.key.registered` / `pix.key.deleted` | key id | see [pix-service](../services/pix-service.md) |
| | `pix.transfer.completed` / `pix.transfer.failed` / `pix.transfer.received` | transfer id | `end_to_end_id`, accounts, counterparty, amount |

## Consumers

| Consumer | Topics | Purpose |
|---|---|---|
| statement-service | `bank.transaction.events`, `bank.account.events` | Customer feed projection |

## Testing

- `packages/go-platform/outbox`: the relay keeps rows on broker failure, publishes in order, never twice in a healthy run; the inbox processes each event once per consumer.
- `tests/e2e`: a transfer's `transaction.completed` and `ledger.entry.posted` reach Kafka with the request's correlation ID.
