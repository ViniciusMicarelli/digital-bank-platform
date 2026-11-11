# ADR-0008: Orchestrated Sagas for Money Movements

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

A Pix transfer, a debit card purchase or a bill payment involves several services and external systems. Distributed transactions (2PC) do not scale and are not supported by Kafka or the external systems.

## Decision

`transaction-service` coordinates multi-step money movements as **orchestrated sagas** with persisted state and explicit compensating actions.

## Alternatives Considered

- **Choreography (pure events)**: no central coordinator, but flows are hard to follow and debug once there are more than a few steps.
- **2PC / XA transactions**: strong consistency, but blocking, fragile and unsupported across our stack.

## Consequences

- ✅ Each flow is visible as an explicit state machine.
- ✅ Recovers from crashes by resuming persisted sagas.
- ⚠️ `transaction-service` becomes a critical path and must be highly available.
- ⚠️ Compensations must be idempotent and carefully designed.
