# Run the local stack

Builds and starts everything with Docker Compose: Postgres, Kafka, the six services, Jaeger and Prometheus.

## Usage

```bash
make up        # stack
make up-obs    # stack + Grafana on http://127.0.0.1:3000
make demo      # walk through the public API (needs jq)
```

A single service can run on the host against the compose infrastructure:

```bash
make run-ledger    # go run ./services/ledger-service/cmd/server
```

## Endpoints

- REST API: http://127.0.0.1:8000
- gRPC: 127.0.0.1:9001 (ledger) … 9005 (statement)
- Jaeger: http://127.0.0.1:16686 · Prometheus: http://127.0.0.1:9090
