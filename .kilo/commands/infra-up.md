# Start local infrastructure

The infrastructure (Postgres with one database per service, Kafka in KRaft mode, Jaeger, Prometheus) is part of the compose stack:

```bash
make up                         # everything
docker compose up -d --wait postgres kafka   # only the infrastructure
```

Postgres listens on 127.0.0.1:54329 (user/password `bank`), Kafka on 127.0.0.1:9094.

## Cleanup

```bash
make down
```
