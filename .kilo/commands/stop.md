# Stop all services

```bash
make down
```

Data in Postgres survives (named volume `pgdata`). To wipe it: `docker compose down -v`.
