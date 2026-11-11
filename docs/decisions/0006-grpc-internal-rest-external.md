# ADR-0006: gRPC Internally, REST Externally

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Internal calls need strong typing, low overhead and generated clients. External clients (web, mobile, third parties) need a widely supported, debuggable API.

## Decision

- **Service ↔ service**: gRPC with Protobuf (`contracts/proto`), managed with `buf`.
- **Client ↔ platform**: REST/JSON exposed by `api-gateway`, specified with OpenAPI 3.1 (`contracts/openapi`).

## Alternatives Considered

- **REST everywhere**: simpler, but no generated type-safe clients and more serialization overhead.
- **GraphQL at the edge**: flexible for clients, but adds complexity around caching, authorization and rate limiting.

## Consequences

- ✅ Type-safe contracts with breaking-change detection (`buf breaking`).
- ✅ Clients consume a conventional, well-documented REST API.
- ⚠️ The gateway must translate REST ↔ gRPC and map errors (see `packages/go-errors`).
