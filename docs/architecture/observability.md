# Observability

All instrumentation is provided by `packages/go-observability` and `packages/go-logger`, so every service emits the same signals.

## The Three Pillars

| Signal | Tool | Notes |
|---|---|---|
| Traces | OpenTelemetry → Jaeger | Context is propagated over HTTP, gRPC and Kafka headers |
| Metrics | OpenTelemetry / Prometheus → Grafana | RED metrics for every endpoint, plus business metrics |
| Logs | `slog` (JSON) → Loki | Every log line carries `trace_id` and `span_id` |

## Standard Metrics

**RED (per endpoint / consumer)**

- `rpc_server_requests_total{service, method, code}`
- `rpc_server_duration_seconds{service, method}`
- `kafka_consumer_lag{topic, group}`
- `outbox_pending_events{service}`

**Business**

- `pix_transfers_total{status}`
- `card_authorizations_total{result, reason}`
- `ledger_entries_total`
- `transaction_amount_cents_sum{type}`

## Logging Conventions

```json
{
  "time": "2026-01-15T13:45:12.123Z",
  "level": "INFO",
  "service": "pix-service",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "msg": "pix transfer completed",
  "transfer_id": "01HQ...",
  "amount_cents": 5000
}
```

- Use structured fields, not string interpolation.
- Never log secrets, tokens, PAN, CVV, passwords or full documents.

## Health Checks

Every service exposes:

- `GET /healthz`: liveness
- `GET /readyz`: readiness (DB, Kafka and dependencies)
- `GET /metrics`: Prometheus metrics

## SLOs

The full list of latency, availability and durability targets lives in [requirements.md](requirements.md). Highlights:

| Flow | SLO |
|---|---|
| Tier 0 availability (ledger, Pix, card authorization, login) | 99.99% |
| Get balance | p99 < 50 ms |
| Card authorization | p99 < 200 ms |
| Pix transfer end-to-end | p95 < 2 s |
| Transaction visible in the feed | p95 < 1 s |

SLOs are tracked with **multi-window burn-rate alerts** (fast burn: 1 h / 5 min, slow burn: 6 h / 30 min).

## Alerts

Alert rules and Grafana dashboards are versioned in `infrastructure/monitoring`:

- Error rate above the SLO budget
- Kafka consumer lag growing
- Outbox backlog growing
- Ledger invariant violation (sum of postings ≠ 0), which is **critical**
- Read replica lag above 500 ms
- Connection pool saturation (PgBouncer / pgxpool)
- HPA at maximum replicas
