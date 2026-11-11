# ADR-0003: Database per Service with PostgreSQL

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Shared databases create hidden coupling between services and prevent independent schema evolution and deployment.

## Decision

Each service owns its own **PostgreSQL** database. Other services access that data only through the owning service's API or events.

## Alternatives Considered

- **Shared database**: easy joins, but tight coupling.
- **NoSQL (DynamoDB / Cassandra)**: great horizontal scale, but weaker transactional guarantees, which the ledger needs.

## Consequences

- ✅ Strong ACID guarantees where they matter most (ledger, outbox).
- ✅ Independent migrations and deployment.
- ⚠️ No cross-service joins. Read models are built in `statement-service` (CQRS).
- ⚠️ Consistency across services is eventual and handled with sagas and events.
