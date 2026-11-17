# Event Documentation

This directory contains event schemas for the Digital Bank Platform.

## Overview

Events drive the architecture of the platform. Services communicate asynchronously by publishing and consuming events via Apache Kafka.

### Event Architecture

```mermaid
flowchart LR
    subgraph Producers
        PROD[All Services]
    end

    subgraph Transactional Outbox
        OUTBOX[(Database Table)]
    end

    subgraph Publisher
        PUBL[Outbox Publisher Process]
    end

    subgraph Kafka
        BUS[(Kafka)]
    end

    subgraph Consumers
        CONS[Event Consumers]
    end

    PROD -.publish-> OUTBOX
    OUTBOX -.poll-> PUBL
    PUBL -.publish-> BUS
    BUS -.deliver-> CONS
```

Every significant state change in the system is captured as a domain event. Events are:
- **Immutable**: Once published, events cannot be modified or deleted
- **Versioned**: Schema versioning is enforced to support schema evolution
- **Idempotent**: Consumers handle duplicate events gracefully

## Event Format

Events are serialized as JSON Schema and published as Protobuf messages for efficiency:

```json
{
  "id": "urn:uuid:<uuid>",
  "type": "AccountOpened",
  "version": 1,
  "createdAt": "2025-01-15T10:30:00Z",
  "payload": {
    "accountId": "acc-12345",
    "customerId": "cust-67890",
    "currency": "BRL",
    "initialBalance": 0.00
  },
  "metadata": {
    "correlationId": "corr-abc-123",
    "source": "account-service",
    "sourceVersion": 1
  }
}
```

## Event Naming Convention

```
{Domain}Event

Examples:
- UserCreated (onboarding)
- UserVerified (identity)
- AccountOpened (accounts)
- AccountClosed (accounts)
- AccountLocked (accounts)
- AccountUnlocked (accounts)
```

## Event Index

| Event ID | Event Name | Type | Producer | Version | Description |
|---------|-----------|------|----------|---------|-------------|
| 001 | UserCreated | Domain | user-service | 1 | A new user account is created |
| 002 | UserVerified | Domain | identity-service | 1 | KYC verification completed |
| 003 | SessionCreated | Domain | identity-service | 1 | New user session started |
| 004 | SessionExpired | Domain | identity-service | 1 | User session expired |
| 005 | AccountOpened | Domain | account-service | 1 | A new checking account is opened |
| 006 | AccountClosed | Domain | account-service | 1 | Account was closed |
| 007 | BalanceCheck | Event | ledger-service | 1 | Balance query executed |
| 008 | LedgerDebit | Domain | ledger-service | 1 | Debit entry created |
| 009 | LedgerCredit | Domain | ledger-service | 1 | Credit entry created |
| 010 | TransferInitiated | Domain | transaction-service | 1 | Transfer request started |
| 011 | TransferSettled | Domain | transaction-service | 1 | Transfer completed |
| 012 | TransferFailed | Domain | transaction-service | 1 | Transfer failed |
| 013 | PixKeyRegistered | Domain | pix-service | 1 | Pix key registered |
| 014 | PixTransferInitiated | Domain | pix-service | 1 | Pix transfer initiated |
| 015 | PixTransferSettled | Domain | pix-service | 1 | Pix transfer settled |
| 016 | PaymentAuthorized | Domain | payment-service | 1 | Payment authorization |
| 017 | PaymentSettled | Domain | payment-service | 1 | Payment settlement |
| 018 | CardIssued | Domain | card-service | 1 | Card issued |
| 019 | CardAuthorized | Domain | card-service | 1 | Card authorization |
| 020 | CardBlocked | Domain | card-service | 1 | Card blocked |
| 021 | CardUnblocked | Domain | card-service | 1 | Card unblocked |
| 022 | CreditLimitUpdated | Domain | credit-service | 1 | Credit limit changed |
| 023 | StatementGenerated | Domain | statement-service | 1 | Statement created |

## Event Catalog

### Onboarding Events

| Event | Schema | Handler |
|-------|--------|--------|
| [UserCreated](./001-user-created.md) | [user-created.jsonschema](./user-created.jsonschema) | user-service |
| [UserVerified](./002-user-verified.md) | [user-verified.jsonschema](./user-verified.jsonschema) | identity-service |
| [SessionCreated](./003-session-created.md) | [session-created.jsonschema](./session-created.jsonschema) | identity-service |

### Account Events

| Event | Schema | Handler |
|-------|--------|--------|
| [AccountOpened](./004-account-opened.md) | [account-opened.jsonschema](./account-opened.jsonschema) | account-service |
| [AccountClosed](./005-account-closed.md) | [account-closed.jsonschema](./account-closed.jsonschema) | account-service |
| [AccountLocked](./006-account-locked.md) | [account-locked.jsonschema](./account-locked.jsonschema) | account-service |
| [AccountUnlocked](./007-account-unlocked.md) | [account-unlocked.jsonschema](./account-unlocked.jsonschema) | account-service |
| [BalanceChecked](./008-balance-checked.md) | [balance-checked.jsonschema](./balance-checked.jsonschema) | ledger-service |

### Ledger Events

| Event | Schema | Handler |
|-------|--------|--------|
| [LedgerDebit](./009-ledger-debit.md) | [ledger-debit.jsonschema](./ledger-debit.jsonschema) | ledger-service |
| [LedgerCredit](./010-ledger-credit.md) | [ledger-credit.jsonschema](./ledger-credit.jsonschema) | ledger-service |
| [LedgerEntryPosted](./011-ledger-entry-posted.md) | [ledger-entry-posted.jsonschema](./ledger-entry-posted.jsonschema) | ledger-service |

### Transaction Events

| Event | Schema | Handler |
|-------|--------|--------|
| [TransferInitiated](./012-transfer-initiated.md) | [transfer-initiated.jsonschema](./transfer-initiated.jsonschema) | transaction-service |
| [TransferSettled](./013-transfer-settled.md) | [transfer-settled.jsonschema](./transfer-settled.jsonschema) | transaction-service |
| [TransferFailed](./014-transfer-failed.md) | [transfer-failed.jsonschema](./transfer-failed.jsonschema) | transaction-service |

### Pix Events

| Event | Schema | Handler |
|-------|--------|--------|
| [PixKeyRegistered](./015-pix-key-registered.md) | [pix-key-registered.jsonschema](./pix-key-registered.jsonschema) | pix-service |
| [PixTransferInitiated](./016-pix-transfer-initiated.md) | [pix-transfer-initiated.jsonschema](./pix-transfer-initiated.jsonschema) | pix-service |
| [PixTransferSettled](./017-pix-transfer-settled.md) | [pix-transfer-settled.jsonschema](./pix-transfer-settled.jsonschema) | pix-service |

### Payment Events

| Event | Schema | Handler |
|-------|--------|--------|
| [PaymentAuthorized](./018-payment-authorized.md) | [payment-authorized.jsonschema](./payment-authorized.jsonschema) | payment-service |
| [PaymentSettled](./019-payment-settled.md) | [payment-settled.jsonschema](./payment-settled.jsonschema) | payment-service |

### Card Events

| Event | Schema | Handler |
|-------|--------|--------|
| [CardIssued](./020-card-issued.md) | [card-issued.jsonschema](./card-issued.jsonschema) | card-service |
| [CardAuthorized](./021-card-authorized.md) | [card-authorized.jsonschema](./card-authorized.jsonschema) | card-service |
| [CardBlocked](./022-card-blocked.md) | [card-blocked.jsonschema](./card-blocked.jsonschema) | card-service |
| [CardUnblocked](./023-card-unblocked.md) | [card-unblocked.jsonschema](./card-unblocked.jsonschema) | card-service |

### Credit Events

| Event | Schema | Handler |
|-------|--------|--------|
| [CreditLimitUpdated](./024-credit-limit-updated.md) | [credit-limit-updated.jsonschema](./credit-limit-updated.jsonschema) | credit-service |
| [CreditInstallmentCreated](./025-credit-installment-created.md) | [credit-installment-created.jsonschema](./credit-installment-created.md) | credit-service |

### Notification Events

| Event | Schema | Handler |
|-------|--------|--------|
| [NotificationTriggered](./026-notification-triggered.md) | [notification-triggered.jsonschema](./notification-triggered.jsonschema) | notification-service |
| [NotificationDelivered](./027-notification-delivered.md) | [notification-delivered.jsonschema](./notification-delivered.jsonschema) | notification-service |

## Event Schema Evolution

Events are versioned. When the schema changes:
- Increment version number
- Backward compatibility: old consumers reject new versions
- Forward compatibility: new consumers accept old versions

## Event Consumers

See service-specific documentation:
- [account-service](../../services/account-service/): Handles account lifecycle events
- [identity-service](../../services/identity-service/): Handles authentication events
- [ledger-service](../../services/ledger-service/): Handles ledger events
- [transaction-service](../../services/transaction-service/): Handles transaction events
- [pix-service](../../services/pix-service/): Handles Pix events
- [payment-service](../../services/payment-service/): Handles payment events
- [card-service](../../services/card-service/): Handles card events
- [credit-service](../../services/credit-service/): Handles credit events
- [statement-service](../../services/statement-service/): Handles statement events
- [notification-service](../../services/notification-service/): Handles notification events

## Testing

Events are tested using:
- Unit tests for handlers: `services/<service>/internal/application/handlers_test.go`
- Integration tests with Testcontainers
- Contract tests with Pact

## Monitoring

Events are monitored via:
- Kafka consumer lag
- Event processing duration
- Failed event count

See [docs/architecture/observability.md](../observability.md) for monitoring setup.
