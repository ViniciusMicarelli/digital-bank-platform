# ADR-0012: Caching Strategy

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Most read traffic hits a small set of hot data (balance display, profile, card list, Pix key lookups). Serving all of it from databases is costly and adds latency.

## Decision

- **Cache-aside** with Redis Cluster for display data, with short TTLs (5–30 s) plus **event-driven invalidation**.
- **In-process caches** for configuration, feature flags and the shard map.
- **Request coalescing** (`singleflight`) and TTL jitter to prevent stampedes.
- **Never** use cached data to decide a money movement.
- Services must keep working, more slowly, when the cache is unavailable.

## Alternatives Considered

- **No cache**: simplest and always consistent, but does not meet p95 targets at 100k RPS without many more replicas.
- **Write-through cache for balances**: fresher data, but couples the write path to Redis availability.

## Consequences

- ✅ Lower latency and database load on hot reads.
- ⚠️ Displayed data can be stale for a few seconds. This is acceptable for display and mitigated by invalidation.
- ⚠️ Cache keys and invalidation rules must be documented per service.
