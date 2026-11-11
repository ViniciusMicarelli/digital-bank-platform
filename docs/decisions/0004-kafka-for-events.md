# ADR-0004: Apache Kafka for Asynchronous Communication

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Many flows (notifications, statements, onboarding steps) do not need synchronous responses and benefit from decoupling. Events must be durable, replayable and ordered per aggregate.

## Decision

Use **Apache Kafka** as the event backbone. Events are partitioned by aggregate ID, and schemas are versioned in `contracts/events`.

## Alternatives Considered

- **RabbitMQ**: flexible routing, but no native log retention or replay.
- **NATS JetStream**: lightweight, but a smaller ecosystem.
- **Cloud-managed queues (SQS/SNS, Pub/Sub)**: fewer operations, but vendor lock-in and harder local development.

## Consequences

- ✅ Replay lets us rebuild projections (statements) from scratch.
- ✅ Per-key ordering guarantees.
- ⚠️ At-least-once delivery requires idempotent consumers (inbox pattern).
- ⚠️ Operating Kafka adds infrastructure complexity (KRaft mode is used locally).
