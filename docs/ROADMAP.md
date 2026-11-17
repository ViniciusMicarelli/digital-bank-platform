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

## Phase 7 — Edge and observability ✅

- `api-gateway`: public REST/JSON API over the internal gRPC services, JWT authentication, `Idempotency-Key` header, consistent error format.
- Structured JSON logs (`log/slog`), Prometheus metrics and OpenTelemetry tracing on every gRPC call.
- Full stack in Docker Compose plus an end-to-end smoke script (`make e2e`).

## Phase 8 — Identity ✅

[identity-service](services/identity-service.md)

- Register and log in with argon2id passwords, lockout after repeated failures, constant-time answers for unknown emails.
- TOTP MFA (RFC 6238) with replay protection.
- Ed25519 access tokens published as a JWKS, with key rotation; the gateway only verifies.
- Rotating refresh tokens with reuse detection (a replayed token revokes the whole session family).
- Private keys and TOTP secrets encrypted at rest (AES-256-GCM, [ADR-0015](decisions/0015-encryption.md)).
- The gateway's development login is removed; e2e and the demo use real sign-up and login.

## Phase 9 — Security hardening ✅

- Mutual TLS 1.3 between services with SPIFFE identities and per-service caller authorization ([ADR-0014](decisions/0014-zero-trust-security.md)).
- CPF encrypted at rest with a blind index, legacy rows encrypted by an expand/contract migration ([ADR-0015](decisions/0015-encryption.md)).
- Rate limits shared across gateway replicas through Redis (GCRA, fail open).

## Phase 10 — Pix QR codes and refunds ✅

- BR Code (EMV) encoder/decoder with CRC16-CCITT; static and one-time charges with expiry.
- Paying QR codes; one-time charges claimed before money moves and released if the payment fails.
- Partial and full refunds of received Pix within 90 days; concurrent refunds bounded by a row lock and a `CHECK` constraint.

## Phase 11 — Debit cards ✅

- card-service: issuing, controls, a vault with encrypted PANs and derived CVVs, real-time authorization (p99 9 ms locally), clearing (partial) and reversal.
- transaction-service: `DEBIT_CARD_PURCHASE`, partial capture and reservation timeouts per type (Pix 15 min, cards 7 days).
- Same-bank Pix can be refunded (the receiver gets an incoming record).
- Every service has a container healthcheck (`/server healthcheck`); dependencies start in order and `make up` waits until all are serving.

## Phase 12 — Operations ✅

- **Ledger reconciliation job**: periodically re-checks every ledger invariant in production and exports `ledger_invariant_violations`, with an alert.
- **Retention jobs**: published outbox rows, processed inbox rows and expired refresh tokens are deleted on a schedule.
- **Contract migration**: drop the plaintext `accounts.document` column, guarded so it refuses to run while unencrypted rows exist.
- **kms-service**: envelope encryption. Services no longer receive key-encryption keys; they hold a data key wrapped by the KMS (per-service key rings, authorized by mTLS identity, versioned master keys with rotation).
- **Certificate rotation**: services reload their certificate from disk on change; `make rotate-certs` reissues short-lived certificates without restarts.
- **Kubernetes**: a Helm chart (Deployments with probes and resources, Services, HPA, PodDisruptionBudgets, default-deny NetworkPolicies that mirror the mTLS caller matrix), validated in CI.
- **Terraform**: AWS reference infrastructure (VPC, EKS, RDS PostgreSQL, MSK, ElastiCache, KMS), validated in CI.

## Phase 13 — Pix: scheduling, recurring and dynamic QR codes ✅

- Scheduled Pix (a date in the future) and recurring Pix (weekly / monthly), executed by a worker, cancellable.
- Pix Automático: a payer authorizes a receiver (mandate with a maximum amount and frequency); the receiver's charges are paid automatically within it.
- Dynamic QR codes: the BR Code carries a URL; the payload is served as a JWS signed by the bank (Ed25519), verified before paying.

## Phase 14 — Cards: step-up and tokenization ✅

- Step-up authentication: revealing card data requires a fresh password (+ TOTP when enabled) confirmation, proven by a short-lived step-up token.
- Tokenization: device tokens (a different card number per wallet) mapped to the card in the vault, suspendable on their own; authorizations by token resolve to the card.

## Phase 15 — Credit

- credit-service: credit limits, credit-function authorizations against the limit, monthly invoices (closing and due dates), invoice payment from the checking account, late interest.
- card-service: the card gains a credit function; the network chooses debit or credit.

## Phase 16 — Bill payments (boleto)

- payment-service: boleto parsing and validation (47-digit line, check digits, due date, amount), payment through a two-phase saga with a simulated settlement network, scheduled payments.

## Phase 17 — Notifications

- notification-service: consumes domain events and notifies customers (in-app inbox, simulated push/email), with per-customer preferences and a live stream (SSE) through the gateway.

## Phase 18 — Onboarding and KYC

- user-service: customer profile (CPF, birth date, address), KYC checks against a simulated bureau (approve, reject, manual review), and accounts can only be opened after KYC approval.
