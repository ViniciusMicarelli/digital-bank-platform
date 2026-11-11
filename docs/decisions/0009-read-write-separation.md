# ADR-0009: Separate Read and Write Databases

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Read traffic (balance screens, feed, card lists) is about 5–10× higher than write traffic and has different access patterns. Serving both from a single primary database limits throughput and lets heavy reads hurt write latency on critical money flows.

## Decision

Separate reads from writes on three levels:

1. **Primary vs read replicas**: every service uses two connection pools (`DB_WRITE_URL`, `DB_READ_URL`) and separates `CommandRepository` (primary) from `QueryRepository` (replicas).
2. **CQRS projections**: `statement-service` builds read-optimized models from events for the feed and statements.
3. **Cache**: Redis for hot display data with short TTLs and event-driven invalidation.

Reads that **decide** something about money (balance before a debit, limits, idempotency) always use the primary.

## Alternatives Considered

- **Single primary for everything**: simplest, but becomes the bottleneck and couples read spikes with write latency.
- **Pure event sourcing with projections for every query**: maximum flexibility, but too complex for most services.

## Consequences

- ✅ Reads scale independently by adding replicas or projection capacity.
- ✅ Write latency on the primary is protected from read spikes.
- ⚠️ Replica lag means eventual consistency on display reads. This is handled with read-your-writes techniques (return the new state on writes, sticky-primary window).
- ⚠️ Developers must choose the right repository. Code review and lint rules enforce it.
