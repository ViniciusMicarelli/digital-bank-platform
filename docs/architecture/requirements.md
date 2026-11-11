# Non-Functional Requirements

These are the targets the platform is designed and tested against. Each architectural decision should be traceable to at least one of these requirements.

## Scale Assumptions

The platform is designed for a bank with **tens of millions of active customers**:

| Metric | Design target | Notes |
|---|---|---|
| Registered customers | 100 M | |
| Daily active users (DAU) | 30 M | |
| Peak concurrent sessions | 5 M | Paydays, Black Friday, 13th salary |
| Peak API requests (read) | 100,000 RPS | Mostly balance, feed and card screens |
| Peak financial writes | 20,000 TPS | Pix, transfers, card purchases, boletos |
| Peak card authorizations | 10,000 TPS | |
| Peak Pix transfers | 8,000 TPS | |
| Ledger postings | ~3 B / month | ~2 postings per movement |
| Events on Kafka | 200,000 msg/s at peak | |
| Growth headroom | 3× peak | Without architectural changes, only more instances or shards |

> These are design numbers, not measured production traffic. Load tests (`k6`) validate them per release.

## Latency Targets

Server-side latency measured **at the API Gateway** (excludes the client's network). These are the SLOs for each endpoint and operation.

### Public API

| Operation | Endpoint | p50 | p95 | p99 |
|---|---|---|---|---|
| Get balance | `GET /v1/accounts/{id}/balance` | 10 ms | 30 ms | 50 ms |
| Transaction feed (first page) | `GET /v1/accounts/{id}/transactions` | 20 ms | 60 ms | 100 ms |
| Profile | `GET /v1/me` | 10 ms | 30 ms | 50 ms |
| List cards | `GET /v1/cards` | 15 ms | 40 ms | 80 ms |
| List invoices | `GET /v1/credit/invoices` | 20 ms | 60 ms | 100 ms |
| Login | `POST /v1/auth/login` | 80 ms | 150 ms | 300 ms |
| Token refresh | `POST /v1/auth/refresh` | 15 ms | 40 ms | 80 ms |
| Pix key lookup | `GET /v1/pix/keys/{key}` | 20 ms | 50 ms | 100 ms |
| Create Pix transfer (accepted) | `POST /v1/pix/transfers` | 60 ms | 150 ms | 300 ms |
| Internal transfer | `POST /v1/transfers` | 40 ms | 100 ms | 200 ms |
| Parse boleto | `POST /v1/payments/boletos/parse` | 15 ms | 50 ms | 100 ms |
| Pay boleto (accepted) | `POST /v1/payments/boletos` | 60 ms | 150 ms | 300 ms |
| Monthly statement PDF | `POST /v1/statements/export` | async | async | async (ready in < 30 s) |

> Argon2id hashing makes login slower than other endpoints on purpose.

### Critical internal paths

| Operation | p50 | p95 | p99 |
|---|---|---|---|
| Card authorization (network → decision) | 40 ms | 100 ms | **200 ms** |
| Pix end-to-end settlement (request → confirmed) | 500 ms | 2 s | 5 s |
| `ledger.PostEntry` | 5 ms | 15 ms | 30 ms |
| Single internal gRPC hop | 2 ms | 10 ms | 20 ms |
| Redis read | < 1 ms | 2 ms | 5 ms |
| Primary DB write transaction | 3 ms | 10 ms | 25 ms |

### Asynchronous propagation

| Flow | p95 | p99 |
|---|---|---|
| Outbox → Kafka publish | 200 ms | 500 ms |
| Transaction → visible in the feed (`statement-service`) | 1 s | 3 s |
| Transaction → push notification delivered to provider | 2 s | 5 s |
| Read replica lag | 100 ms | 500 ms |

### Latency budget example: card authorization (p99 = 200 ms)

| Step | Budget |
|---|---|
| Network ingress + TLS | 20 ms |
| `card-service`: card status, controls, vault lookup | 30 ms |
| Fraud rules engine | 50 ms |
| `credit-service.ReserveLimit` or `ledger.Hold` | 50 ms |
| Persist authorization + outbox | 20 ms |
| Safety margin | 30 ms |

Every synchronous chain must fit its budget. A new hop must take its time from the existing budget.

## Availability

| Tier | Services / flows | Availability | Max downtime / month |
|---|---|---|---|
| **Tier 0** (money critical) | ledger, transaction, card authorization, Pix, identity (login/refresh), gateway | **99.99%** | ~4.3 min |
| **Tier 1** | account, user, payment, credit, statement (feed) | 99.95% | ~22 min |
| **Tier 2** | notifications, exports, analytics | 99.9% | ~43 min |

## Durability and Recovery

| Data | RPO (data loss) | RTO (recovery time) |
|---|---|---|
| Ledger, transactions, Pix, card authorizations | **0** (synchronous replication across AZs) | < 1 min (AZ failure), < 15 min (region failure) |
| Other transactional data | < 1 s | < 5 min |
| Read models (statements, feed) | Rebuildable from Kafka / ledger | < 1 h for a full rebuild |
| Kafka | 0 (`acks=all`, `min.insync.replicas=2`) | < 1 min |

## Consistency per Domain

| Domain | Model | Why |
|---|---|---|
| Ledger balances used for decisions | **Strong** (primary only) | Prevents overdraft and double spending |
| Limits checked before a transaction | **Strong** | Same reason |
| Feed, statements, notifications | **Eventual** (seconds) | Display only |
| Displayed balance | Read-your-writes | The customer must see the effect of their own action right away |
| Cross-service flows | Eventual + sagas | No distributed transactions |

## Other Requirements

| Area | Requirement |
|---|---|
| Scalability | Every service scales **horizontally** (stateless pods). No vertical scaling to meet growth |
| Throughput per pod | Each service pod handles at least 2,000 RPS of its main read operation |
| Deployments | Zero downtime (rolling / canary), at least 10 deploys per day possible |
| Backward compatibility | Contracts and DB schemas compatible with N-1 |
| Security | See [security.md](security.md) |
| Observability | 100% of requests traced (tail-based sampling for storage), see [observability.md](observability.md) |
| Cost | Autoscaling down outside peak hours, with a minimum of 3 replicas per AZ-spread service |

## How We Verify

- **Load tests** (`k6`) run nightly against a staging environment at 1× peak, and before major releases at 3× peak.
- **Soak tests** run for 12 h at average load to detect leaks.
- **Chaos tests** (Chaos Mesh): kill pods, kill a DB primary, add network latency, lose an AZ.
- **SLO dashboards and burn-rate alerts** in Grafana ([observability.md](observability.md)).
