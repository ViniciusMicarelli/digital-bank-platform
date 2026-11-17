# ADR-0016: Idempotency Keys with Request Fingerprints

- **Status:** Accepted
- **Date:** 2026-11-17

## Context

Networks drop responses. A client that times out on "send R$ 100" cannot know whether the money moved, and neither can a saga whose downstream call timed out. Retrying must be safe, and must never move money twice.

## Decision

Every operation that moves money or creates something is **idempotent on a caller-provided key**:

- Public API: an `Idempotency-Key` header, scoped to the customer (`<customer>:<key>`).
- Internal calls derive keys from the parent ID: `txn:<id>:entry`, `txn:<id>:hold`, `pix:<transfer id>`, `pix-in:<E2E id>`; limit usage is keyed by transaction ID.
- Each store saves a SHA-256 **fingerprint** of the request next to the key. Same key + same fingerprint returns the original result; same key + different fingerprint fails with `IDEMPOTENCY_CONFLICT`.
- The key is claimed with `INSERT … ON CONFLICT DO NOTHING` in the same transaction as the effect, so a failed attempt rolls back the claim and the key can be retried.
- Account IDs are UUIDv5 of the opening key, so a retried `OpenAccount` targets the same ledger account.

## Alternatives Considered

- **Deduplicate by time window in a cache**: loses keys on eviction and cannot return the original result.
- **Exactly-once messaging**: does not cover synchronous gRPC/HTTP calls, and Kafka's transactions do not span Postgres.

## Consequences

- ✅ Any step of any saga can be replayed; recovery workers only move forward.
- ✅ Clients get a clear contract: retry with the same key until you get a final answer.
- ⚠️ Keys are stored forever in this version; production needs a retention policy (e.g. 30 days) per table.
