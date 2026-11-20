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

## Phase 15 — Credit ✅

- credit-service: credit limits, credit-function authorizations against the limit, monthly invoices (closing and due dates), invoice payment from the checking account, late interest.
- card-service: the card gains a credit function; the network chooses debit or credit.

## Phase 16 — Bill payments (boleto) ✅

- payment-service: boleto parsing and validation (47-digit line, check digits, due date, amount), payment through a two-phase saga with a simulated settlement network, scheduled payments.

## Phase 17 — Notifications ✅

- notification-service: consumes domain events and notifies customers (in-app inbox, simulated push/email), with per-customer preferences and a live stream (SSE) through the gateway.

## Phase 18 — Onboarding and KYC ✅

- user-service: customer profile (CPF, birth date, address), KYC checks against a simulated bureau (approve, reject, manual review), and accounts can only be opened after KYC approval.

## Phase 19 — Web app ✅

- Customer web app (`apps/web`): onboarding with KYC, account, Pix (send, QR Code request and payment, keys, refunds), cards (step-up, controls, wallets, simulated purchases), credit and invoices, boletos, notifications streamed live, settings (TOTP, limits).
- Served by nginx with the API behind it (one origin, CSP); Playwright browser tests on desktop and mobile run in CI against that image.
- Found on the way: a client that disconnects mid-request no longer cancels the write it started (the gateway detaches non-GET requests from the client, with a 30 s bound).

## Phase 20 — Mobile app ✅

- iOS and Android app (`apps/mobile`, Expo SDK 58, React Native, Expo Router): onboarding with KYC, home with pull to refresh, Pix (send, keys, QR Code request and share, copia e cola), cards (step-up sheet, controls, simulated purchases), boletos, credit, notifications and preferences.
- `packages/ts-core`: the API client, money, SSE parser, errors and types shared with the web app; the SSE transport is fetch streaming on the web and incremental XMLHttpRequest on React Native (its fetch does not stream).
- Refresh token in the Keychain / Keystore (`expo-secure-store`, this device only); the phone finds the gateway at the Expo dev server's address.
- The gateway allows listed browser origins (`CORS_ALLOWED_ORIGINS`) so the app's web build can call it; CI lints, typechecks and runs Playwright on that build at a phone viewport.

## Phase 21 — Design system, themes and motion ✅

- One design language for web and mobile: semantic color tokens with light and dark values (jade for money in and the brand, neutral for money out, red only for failures, amber for pending, violet for credit), Inter, Lucide icons.
- Light and dark themes in both apps, following the system or chosen by the customer.
- Apple-style components over Liquid Glass (native `GlassView` on iOS 26, blur elsewhere) and an aurora background; the familiar digital-bank home (round shortcuts, my cards, invoice, activity).
- Realistic cards (chip, contactless, sheen, 3D flip to the back after step-up), balance and limit count-ups, staggered entrances, skeletons; GSAP on the web, Reanimated on mobile, both respecting reduced motion.
- Browser tests run in both themes.

## Phase 22 — Back office ✅

- Staff role (`ANALYST`) in identity-service, carried in the access token; the gateway's back-office routes require it and log every request with the analyst.
- KYC manual review queue and decisions with the analyst recorded; customer file (profile, verification, accounts with balances); account block (with a reason) and unblock.
- Back-office area in the web app (`/backoffice`): queue, customer file, decision, account actions; analysts land there after signing in, customers cannot open it.

## Phase 23 — Back office: MFA, audit trail and four-eyes ✅

- Access tokens say how the session was authenticated (`amr`); the back office requires a second factor (`MFA_REQUIRED`) and the web app walks an analyst through TOTP enrollment.
- backoffice-service: append-only, hash-chained audit trail with chain verification; the gateway writes every staff action (and every look at a customer's file) before it runs and fails closed.
- Four-eyes approvals: unblocking an account needs a second analyst; approved actions run right away and are retried after an outage; requests expire after 24 h.
- Web back office: approvals panel, unblock requests, audit trail with the chain status on the customer file. Two development analysts with TOTP; the e2e and browser tests share the last TOTP step used, so no code is replayed.

## Phase 24 — Card refunds and disputes ✅

- Merchant refunds of cleared purchases from the card network, idempotent and never more than what is left; debit refunds go back to the checking account (`CARD_REFUND`), credit refunds become a credit on the invoice, carried to the next invoice when larger than it.
- Disputes: the customer disputes a purchase within 90 days; "not recognized" blocks the card for good; an analyst accepts (the amount is refunded) or rejects with a reason in the back office, audited.
- Refunds left pending by an outage are retried; notifications for disputes and refunds; refunds in the statement.
- Web and mobile: the card's purchases with "Contestar"; back-office disputes panel.

## Phase 25 — Performance on phones and a calmer look ✅

- No animated aurora and no blur per card (they made phones drop frames until taps stopped working): solid surfaces with hairline borders over a plain background; only the two bars blur. Card skins and hero are clean jade / violet gradients.
- Full-bleed on phones: `viewport-fit=cover`, safe areas, the page color behind the browser bars (`theme-color` follows the theme). Page reveals animate opacity only, once.
- Mobile: the Expo dev server forwards the API, so a phone that loads the app always reaches it; demo seed (`make seed`), analyst codes (`make totp`) and a testing guide.

## Phase 26 — Savings boxes (caixinhas) ✅

- savings-service: each box its own ledger account; goals; deposits and withdrawals through transaction-service (`SAVINGS_*`, no transfer limits), idempotent, recovered after outages; close returns everything.
- Daily compounded yield on the balance at the end of each Brasília day, paid exactly once from the ledger's `savings-yield` account, the fraction of a centavo carried; sandbox to simulate days.
- Web and mobile: *Caixinhas* screen (boxes, goal progress, save / withdraw, history, simulate a month), on the home and in the shortcuts; savings in the statement.

## Phase 27 — Installment purchases ✅

- Credit purchases in 2 to 12 interest-free installments (at least R$ 5,00 each): the whole amount on the limit, paid to the merchant at clearing; one installment per invoice from the capture invoice on, the first with the remainder centavo; upcoming installments on the line and a list of plans.
- Web and mobile: choose the installments when simulating a credit purchase, "em 6x" on the card's purchases, *Compras parceladas* on the credit screen with what is still to come.

## Phase 28 — Spending by category ✅

- statement-service projects spending from card, transaction and refund events: card purchases categorized by merchant category code, Pix and transfers sent, bills paid; refunds subtract; month in Brasília time with the previous month and the top merchants.
- Card events now name the account on clearing and reversal; older events are placed through the card.
- Web and mobile: *Gastos* screen (month by month, donut / bar by category, change from last month, where the most was spent).

## Phase 29 — Minimum payment and invoice plans ✅

- Every invoice shows its minimum payment (15%, at least R$ 50).
- A closed invoice can be split into 2 to 12 fixed installments with interest (Price table, 4.99% a month), with the total, the interest and the CET shown before confirming; the invoice is settled by the plan, the installments follow on the next invoices, the interest is booked in the ledger.
- Web and mobile: minimum payment and *Parcelar fatura* on the invoice to pay.

## Phase 30 — Credit limit changes ✅

- Customers ask for a new limit: decreases at once (never below what is used), increases up to +50% at once, refused with an overdue invoice, larger ones to an analyst who may grant less or refuse with a reason (audited); notifications for each outcome.
- Web and mobile: *Ajustar limite* on the credit screen with the history of requests; back-office *Limites* queue.

## Phase 31 — Receipts and statement export ✅

- A receipt for every statement item with an authentication code signed by the bank (HMAC of what it states), checked publicly at `/comprovante/<code>` with names masked; print / save as PDF and share.
- CSV export of a month (or any period up to a year) for spreadsheets in Portuguese.
- Web: tap any movement for its receipt, *Exportar CSV* on the statement, public check page. Mobile: receipt screen with the system share sheet.

## Phase 32 — Pix contacts ✅

- Recent receivers come from the account's completed Pix (nothing copied); favorites with a nickname on top; one tap fills the key and looks it up.
- Web and mobile: contacts row on *Enviar Pix* and a star to favorite the receiver.

## Phase 33 — Temporary cards ✅

- Single-use virtual cards for online purchases: valid for 24 hours, cancelled by their first approved purchase (claimed atomically), given back when that purchase is declined, never put in a wallet; at most three live.
- Web and mobile: create one from the cards screen, its own look, and what state it is in; mobile now switches between cards.

## Phase 34 — Scheduled deposits ✅

- Savings boxes can take a monthly deposit from the checking account on a chosen day (1–28): exactly once per month, kept due through an outage, skipped (and shown) when the money is not there.
- Web and mobile: set, change and remove it on the box; the box card shows it; the sandbox runs the next one now.

## Phase 35 — Credit score ✅

- The bank's own behaviour score (0–1000) for the card, explained factor by factor: invoices paid on time or late, an overdue invoice, how much of the limit is used, time as a customer, invoice plans.
- It drives the automatic limit analysis (+50%, +100% from 750, refused under 250) and is kept on each request for the analyst.
- Web and mobile: the score, its band and what helps or hurts it on the credit screen; back office shows it on limit requests.

## Phase 36 — Budgets ✅

- Monthly budgets on the month's total or per spending category, with a notification at 80% and at 100%, once each per month; statement-service now publishes through its own outbox.
- Web and mobile: budgets on the spending screen with how much of each is used.

## Next

- Store builds (EAS) with push notifications (APNs / FCM) for the notification-service.
- Back office: staff SSO (OIDC) with WebAuthn, more four-eyes actions; card representment and arbitration.
