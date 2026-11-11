# ADR-0010: Shard by Customer and Deploy in Cells

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

A single PostgreSQL primary per service cannot handle 20k+ financial TPS and billions of postings per month. A single deployment also means that one bad release or database failure affects every customer.

## Decision

- Use `customer_id` as the **partition key** for databases, Kafka and caches.
- Split the key space into **4,096 logical shards**, mapped to physical database clusters through a cached shard directory.
- Group shards into **cells**: independent copies of the customer-scoped stack, each with a known, load-tested capacity.
- Keep a small set of **global services** for lookups that cannot be partitioned by customer (login by CPF, Pix key → account, card BIN routing).

## Alternatives Considered

- **Vertical scaling only**: hits hardware limits and increases blast radius.
- **Distributed SQL (CockroachDB / YugabyteDB / Citus)**: transparent sharding, but adds operational and latency costs and less control over data placement. It stays an option for specific services.
- **Sharding without cells**: scales the data layer but keeps a global blast radius for deployments.

## Consequences

- ✅ Near-linear scaling: more customers → more shards and cells.
- ✅ Failures and deploys are contained to one cell.
- ⚠️ Cross-customer operations (transfers between cells) must go through sagas and events. They never use cross-shard transactions.
- ⚠️ Rebalancing shards requires migration tooling.
- ⚠️ Early phases run with a single cell and a few physical shards. The abstraction exists from day one, so scaling later needs no rewrite.
