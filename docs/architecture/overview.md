# Architecture Overview

## Principles

1. **The ledger is the source of truth.** Balances are never stored as a mutable field. They are derived from immutable double-entry postings.
2. **Each service owns its data.** One database per service, and other services never access it directly.
3. **Async by default, sync when necessary.** Commands that need an immediate answer, such as a balance check or a card authorization, use gRPC. Everything else reacts to events.
4. **Every write is idempotent.** Retries are expected at every layer.
5. **Contracts first.** Protobuf, OpenAPI and event schemas in `contracts/` are written before the implementation.
6. **Observable by design.** Every request carries a trace ID from the client to the database.
7. **Horizontally scalable.** Stateless services, separate read and write paths, and data partitioned by customer. See [scalability.md](scalability.md).
8. **Measurable targets.** Every flow has a latency budget in milliseconds. See [requirements.md](requirements.md).

## System Context

```mermaid
flowchart TB
    Customer((Customer))
    Customer --> Apps[Web / Mobile Apps]
    Apps --> Platform[Digital Bank Platform]

    Platform <--> BCB[Central Bank: Pix SPI / DICT<br/><i>simulated</i>]
    Platform <--> Network[Card Network<br/><i>simulated</i>]
    Platform <--> Bureau[KYC / Credit Bureau<br/><i>simulated</i>]
    Platform --> Providers[Email / SMS / Push Providers]
```

External integrations are implemented as **simulators** behind ports (interfaces), so they can be replaced by real adapters without changing domain code.

## Containers

```mermaid
flowchart LR
    GW[api-gateway]

    subgraph Identity
        IDN[identity-service]
        USR[user-service]
    end

    subgraph Core Banking
        ACC[account-service]
        LED[ledger-service]
        TRX[transaction-service]
    end

    subgraph Payments
        PIX[pix-service]
        PAY[payment-service]
    end

    subgraph Cards & Credit
        CRD[card-service]
        CRE[credit-service]
    end

    subgraph Read Side
        STM[statement-service]
        NOT[notification-service]
    end

    GW --> IDN & USR & ACC & TRX & PIX & PAY & CRD & CRE & STM
    PIX & PAY & CRD --> TRX
    TRX --> LED
    CRD --> CRE
    Kafka[(Kafka)]
    IDN & USR & ACC & LED & TRX & PIX & PAY & CRD & CRE --> Kafka
    Kafka --> STM & NOT
```

| Bounded context | Services |
|---|---|
| Identity | `identity-service`, `user-service` |
| Core Banking | `account-service`, `ledger-service`, `transaction-service` |
| Payments | `pix-service`, `payment-service` |
| Cards & Credit | `card-service`, `credit-service` |
| Read Side | `statement-service`, `notification-service` |

## Communication

| Type | Technology | When to use |
|---|---|---|
| Client → Platform | REST/JSON over HTTPS | All client traffic, only through the gateway |
| Service → Service (sync) | gRPC over mTLS | Queries and commands that need an immediate result |
| Service → Service (async) | Kafka events | State changes, integrations and projections |

## Main Flows

### Customer onboarding

```mermaid
sequenceDiagram
    participant App
    participant GW as api-gateway
    participant USR as user-service
    participant IDN as identity-service
    participant ACC as account-service
    participant LED as ledger-service
    participant K as Kafka

    App->>GW: POST /v1/users
    GW->>USR: CreateUser
    USR->>K: user.registered
    K->>IDN: create credentials
    USR->>USR: run KYC (async)
    USR->>K: user.kyc.approved
    K->>ACC: open checking account
    ACC->>LED: CreateLedgerAccount
    ACC->>K: account.opened
    K-->>App: push notification (via notification-service)
```

### Pix transfer (outgoing)

```mermaid
sequenceDiagram
    participant App
    participant GW as api-gateway
    participant PIX as pix-service
    participant TRX as transaction-service
    participant LED as ledger-service
    participant SPI as SPI (simulated)
    participant K as Kafka

    App->>GW: POST /v1/pix/transfers (Idempotency-Key)
    GW->>PIX: CreateTransfer
    PIX->>PIX: resolve key in DICT
    PIX->>TRX: StartTransaction
    TRX->>LED: Post (debit customer, credit pix-settlement)
    LED-->>TRX: posted
    TRX-->>PIX: reserved
    PIX->>SPI: send payment order
    alt settled
        SPI-->>PIX: confirmed
        PIX->>TRX: Complete
        PIX->>K: pix.transfer.completed
    else rejected
        SPI-->>PIX: rejected
        PIX->>TRX: Compensate
        TRX->>LED: Post (reversal)
        PIX->>K: pix.transfer.failed
    end
```

### Card purchase authorization

```mermaid
sequenceDiagram
    participant NET as Card Network (simulated)
    participant CRD as card-service
    participant CRE as credit-service
    participant TRX as transaction-service

    NET->>CRD: Authorization request
    CRD->>CRD: validate card status, CVV, limits, fraud rules
    alt credit
        CRD->>CRE: ReserveLimit
    else debit
        CRD->>TRX: StartTransaction (hold)
    end
    CRD-->>NET: approved / declined (< 200 ms)
```

## Deployment

- **Local**: Docker Compose (`infrastructure/docker`)
- **Cloud**: Kubernetes (`infrastructure/kubernetes`) provisioned with Terraform (`infrastructure/terraform`)
- Each service is a stateless container that scales horizontally. State lives in PostgreSQL, Redis and Kafka.
- Production runs across 3 availability zones, with a cell-based layout for large scale. See [scalability.md](scalability.md#cell-based-architecture).
