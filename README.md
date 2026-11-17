# Digital Bank Platform

A cloud-native digital banking platform built in **Go** with a microservices architecture: checking accounts, a double-entry ledger, Pix instant payments, cards, bill payments and statements.

![Status](https://img.shields.io/badge/status-work%20in%20progress-yellow)
![Go](https://img.shields.io/badge/go-1.26+-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)

> **Status:** in progress. The code is built one vertical slice at a time, following the [phased roadmap](docs/ROADMAP.md).

## Architecture at a glance

- **Double-entry ledger as the source of truth.** Balances are derived from immutable, balanced postings ([ADR-0005](docs/decisions/0005-double-entry-ledger.md)).
- **Database per service**, with no shared tables ([ADR-0003](docs/decisions/0003-database-per-service.md)).
- **Events over Kafka**, published through a transactional outbox ([ADR-0004](docs/decisions/0004-kafka-for-events.md), [ADR-0007](docs/decisions/0007-transactional-outbox.md)).
- **Orchestrated sagas** for multi-step money movements like Pix ([ADR-0008](docs/decisions/0008-orchestrated-sagas.md)).
- **gRPC inside, REST at the edge** ([ADR-0006](docs/decisions/0006-grpc-internal-rest-external.md)).
- **Hexagonal services**: `domain` → `application` → `ports` → `adapters`.

Full documentation: [docs/README.md](docs/README.md). Decisions: [docs/decisions](docs/decisions/README.md).

## Repository layout

```
.
├── services/        # one Go module per bounded context (hexagonal layout)
├── packages/        # shared Go libraries
│   └── go-common/   #   money: exact, currency-aware amounts in minor units
├── contracts/       # protobuf, OpenAPI and event schemas
├── infrastructure/  # docker, kubernetes, terraform, monitoring
├── apps/            # web and mobile clients
├── docs/            # architecture, services, ADRs
└── go.work          # ties all Go modules together
```

## Getting started

Requirements: Go 1.26+, Docker.

```bash
make help    # list targets
make test    # run all tests with the race detector
make lint    # gofmt + go vet
```

## Roadmap

- [x] Architecture docs and ADRs
- [x] `packages/go-common/money`: exact money type, no floats, overflow-checked
- [x] **ledger-service**: journal entries, balanced postings, holds, reversals, idempotency (PostgreSQL + gRPC)
- [x] **account-service**: account lifecycle, CPF validation, per-transaction/daily/nightly limits
- [ ] Local stack with Docker Compose + migrations + integration tests
- [ ] Internal transfers end to end (account → ledger → statement)
- [ ] Transactional outbox → Kafka
- [ ] **pix-service** with an orchestrated saga
- [ ] API gateway, identity and observability (OpenTelemetry)

## License

[MIT](LICENSE)
