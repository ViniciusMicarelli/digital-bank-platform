# Start local infrastructure

Starts all infrastructure services required for local development.

## Usage

```bash
make infra-up
```

## Services

- PostgreSQL (primary and replica)
- Kafka (message broker)
- Redis (caching)
- Prometheus (metrics)
- Grafana (visualization)
- Loki (logging)
- Jaeger (tracing)

## Cleanup

```bash
make infra-down
```
