# ADR-0002: Microservices in a Monorepo

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Banking domains (accounts, ledger, Pix, cards, credit) have different scaling needs, failure modes and rates of change. At the same time, contracts and shared libraries must evolve together.

## Decision

Split the system into **microservices aligned with bounded contexts**, all hosted in a **single repository**:

- `services/`: one folder per service, each with its own `go.mod`.
- `packages/`: shared Go libraries.
- `contracts/`: Protobuf, OpenAPI and event schemas.
- A root `go.work` ties modules together for local development.

## Alternatives Considered

- **Modular monolith**: simpler to operate, but it does not show independent scaling and deployment.
- **Polyrepo**: strong isolation, but contract changes become hard to coordinate.

## Consequences

- ✅ Atomic changes across contracts and services.
- ✅ Consistent tooling, CI and conventions.
- ⚠️ CI must detect which services changed so it only builds and tests what is affected.
- ⚠️ Operational complexity (networking, observability, distributed consistency) is real and handled by the patterns in [patterns.md](../architecture/patterns.md).
