# Roadmap

The platform is built in **vertical slices**: each phase ends with working, tested code that runs locally, instead of scaffolding every service at once. Each phase is one or more commits and is merged only when `make lint test` (and the integration tests, from Phase 1 on) pass.

## Phase 0 — Foundation ✅

- Clean repository, Go workspace (`go.work`), Makefile, CI.
- `packages/go-common/money`: exact amounts in int64 minor units, no floats.

## Phase 1 — Ledger core ✅

The source of truth for money ([ADR-0005](decisions/0005-double-entry-ledger.md), [ledger-service](services/ledger-service.md)).

- `contracts/`: protobuf definitions, generated with `buf`, as their own Go module.
- `packages/go-platform`: shared runtime pieces (Postgres pool + embedded migrations, test databases, gRPC server bootstrap, config).
- `ledger-service`:
  - Domain: ledger accounts, journal entries with balanced postings (sum = 0), holds, reversals.
  - Postgres: `ledger_accounts`, `balances`, `journal_entries`, `postings`, `holds`, `outbox`, plus a deferred constraint trigger that rejects unbalanced entries at the database level.
  - Concurrency: `SELECT … FOR UPDATE` in a fixed lock order; idempotency keys with request fingerprints.
  - gRPC API: `CreateLedgerAccount`, `PostEntry`, `ReverseEntry`, `Hold` / `Capture` / `Release`, `GetBalance`, `GetBalanceAt`, `ListPostings`.
  - Tests: domain unit tests, integration tests on a real Postgres, a concurrency test (N parallel debits never overdraw), and a randomized invariant test (the global sum stays zero).
- `docker-compose.yml` with Postgres (one database per service).

**Done when:** you can post entries over gRPC, and the invariant and concurrency tests pass against Postgres.

## Phase 2 — Accounts ✅

[account-service](services/account-service.md)

- Account state machine (`ACTIVE ⇄ BLOCKED → CLOSED`), branch + account number, limits (per transaction, daily, night time).
- `OpenAccount` creates the ledger account (idempotent), `GetBalance` proxies to the ledger, `CloseAccount` requires a zero balance.
- `CheckLimit` with daily usage tracking.
- Events (`account.opened`, `account.blocked`, …) written to the outbox.

## Phase 3 — Money movement (sagas) ✅

[transaction-service](services/transaction-service.md), [ADR-0008](decisions/0008-orchestrated-sagas.md)

- Orchestrated saga with a persisted step log (`transactions`, `saga_steps`).
- `INTERNAL_TRANSFER`: check limits → post entry.
- Two-phase types (`PIX_OUT`): check limits → hold → external step → capture / release.
- Recovery worker that resumes or compensates stuck sagas.
- End-to-end test: open two accounts, fund one, transfer, check both balances.

## Phase 4 — Events ✅

[ADR-0004](decisions/0004-kafka-for-events.md), [ADR-0007](decisions/0007-transactional-outbox.md)

- Kafka (KRaft, single node) in Docker Compose.
- `packages/go-platform/outbox`: a relay that publishes outbox rows to Kafka (`FOR UPDATE SKIP LOCKED`, at-least-once delivery).
- Common event envelope (id, type, version, occurred_at, correlation_id, payload).
- Consumer helper with an inbox table for idempotent handling.

## Phase 5 — Pix ✅

[pix-service](services/pix-service.md)

- Pix keys (CPF, email, phone, random EVP), max 5 per account, validation.
- Simulated DICT (key directory) and SPI (settlement) behind ports.
- Outgoing transfer: `CREATED → FUNDS_RESERVED → SENT_TO_SPI → COMPLETED | FAILED` through transaction-service.
- Incoming transfers from the simulated SPI. End-to-end ID (E2E ID) per transfer.

## Phase 6 — Statements (read side) ✅

[statement-service](services/statement-service.md)

- Consumes `ledger.entry.posted` and `pix.transfer.*`, builds a per-account feed projection.
- Cursor-paginated `ListFeed`, rebuildable by replaying topics.

## Phase 7 — Edge and observability

- `api-gateway`: public REST/JSON API over the internal gRPC services, JWT authentication, `Idempotency-Key` header, consistent error format.
- Structured JSON logs (`log/slog`), Prometheus metrics and OpenTelemetry tracing on every gRPC call.
- Full stack in Docker Compose plus an end-to-end smoke script (`make e2e`).

## Later (not planned yet)

Cards and card authorization, credit and invoices, boleto payments, notifications, user onboarding/KYC, Kubernetes manifests, Terraform. Each will reuse the ledger holds, sagas and outbox built above.
