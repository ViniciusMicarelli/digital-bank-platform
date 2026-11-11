# ADR-0001: Use Go as the Primary Backend Language

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

The platform consists of many network-heavy services that need low latency (card authorization < 200 ms), high concurrency, small memory footprint and simple deployment.

## Decision

All backend services and shared libraries are written in **Go**.

## Alternatives Considered

- **Java / Kotlin (Spring)**: mature ecosystem, but heavier runtime, slower startup and more memory per pod.
- **Clojure**: strong fit for immutable financial data, but a smaller talent pool and tooling ecosystem.
- **Node.js**: fast to iterate, but weaker type safety and CPU-bound performance.

## Consequences

- ✅ Single static binary, fast startup, small distroless images.
- ✅ Goroutines make concurrent I/O (Kafka consumers, gRPC) straightforward.
- ✅ First-class gRPC, Protobuf and OpenTelemetry support.
- ⚠️ More boilerplate for domain modeling. This is mitigated by shared packages and code generation.
