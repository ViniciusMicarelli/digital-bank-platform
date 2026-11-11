# Event Catalog

Domain events are published on **Apache Kafka** through the [transactional outbox](../architecture/patterns.md#transactional-outbox). Schemas live in `contracts/events/`.

## Conventions

### Topic naming

```text
<domain>.<entity>.<event>.v<version>

pix.transfer.completed.v1
ledger.entry.posted.v1
```

- Events are named in the **past tense**: they describe facts that already happened.
- Partition key: the aggregate ID (for example `account_id`), which guarantees ordering per aggregate.
- Each topic has a dead-letter companion: `<topic>.dlq`.

### Envelope

All events share a [CloudEvents](https://cloudevents.io/)-inspired envelope:

```json
{
  "id": "01HQ8Z3K5Y8V2N4T6R1M0P9QWE",
  "type": "pix.transfer.completed",
  "version": 1,
  "source": "pix-service",
  "subject": "01HQ8Z2...transfer_id",
  "occurred_at": "2026-01-15T13:45:12.123Z",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "data": {
    "transfer_id": "01HQ8Z2...",
    "account_id": "01HQ7A...",
    "amount": { "value": 5000, "currency": "BRL" },
    "end_to_end_id": "E12345678202601151345abcdefghijk"
  }
}
```

### Delivery guarantees

- **At-least-once** delivery. Consumers **must** be idempotent and deduplicate by `id` using the inbox table.
- Ordering is only guaranteed per partition key.
- Schema evolution must be backward compatible: only add optional fields. Breaking changes require a new `v2` topic.

## Catalog

### Identity

| Event | Producer | Consumers |
|---|---|---|
| `user.registered` | user-service | identity, notification |
| `user.kyc.approved` | user-service | account, notification |
| `user.kyc.rejected` | user-service | notification |
| `user.profile.updated` | user-service | statement |
| `identity.login.succeeded` | identity-service | notification |
| `identity.device.registered` | identity-service | notification |

### Core Banking

| Event | Producer | Consumers |
|---|---|---|
| `account.opened` | account-service | pix, card, statement, notification |
| `account.blocked` | account-service | card, pix, notification |
| `account.closed` | account-service | card, pix, credit, statement |
| `ledger.entry.posted` | ledger-service | statement |
| `transaction.initiated` | transaction-service | statement |
| `transaction.completed` | transaction-service | statement, notification |
| `transaction.failed` | transaction-service | statement, notification |

### Payments

| Event | Producer | Consumers |
|---|---|---|
| `pix.key.registered` | pix-service | notification |
| `pix.key.deleted` | pix-service | notification |
| `pix.transfer.completed` | pix-service | statement, notification |
| `pix.transfer.failed` | pix-service | statement, notification |
| `pix.transfer.received` | pix-service | statement, notification |
| `payment.boleto.paid` | payment-service | statement, notification |
| `payment.scheduled.executed` | payment-service | notification |

### Cards and Credit

| Event | Producer | Consumers |
|---|---|---|
| `card.issued` | card-service | notification |
| `card.blocked` | card-service | notification |
| `card.authorization.approved` | card-service | credit, statement, notification |
| `card.authorization.declined` | card-service | notification |
| `credit.limit.changed` | credit-service | card, notification |
| `credit.invoice.closed` | credit-service | statement, notification |
| `credit.invoice.paid` | credit-service | statement, notification |
| `credit.invoice.overdue` | credit-service | card, notification |
