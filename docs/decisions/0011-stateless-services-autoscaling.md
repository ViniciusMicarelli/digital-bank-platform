# ADR-0011: Stateless Services with Horizontal Autoscaling

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Traffic varies strongly by time of day and has predictable peaks (paydays, Black Friday). The platform must absorb peaks without manual work and save cost outside them.

## Decision

- Services are **stateless**. All state lives in PostgreSQL, Redis or Kafka.
- **HPA** scales gRPC/HTTP services on CPU and RPS. **KEDA** scales consumers on Kafka lag. Node autoscaling uses Karpenter or Cluster Autoscaler.
- Every service runs at least 3 replicas spread across 3 AZs, with a PodDisruptionBudget.
- Scheduled pre-scaling runs before known peak events.

## Alternatives Considered

- **Fixed capacity provisioned for peak**: simple, but expensive and still fails on unexpected peaks.
- **Serverless functions**: elastic, but cold starts and connection limits hurt p99 latency and database pooling.

## Consequences

- ✅ Capacity follows demand automatically.
- ✅ Any pod can be killed at any time (safe deploys, chaos testing).
- ⚠️ Scale-up is not instant. Headroom (60% CPU target) and pre-scaling cover the gap.
- ⚠️ Database connections must be pooled (PgBouncer) so a scale-out does not exhaust them.
