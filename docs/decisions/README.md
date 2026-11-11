# Architecture Decision Records

We record significant architectural decisions using [ADRs](https://adr.github.io/), following the template in [0000-template.md](0000-template.md).

| # | Title | Status |
|---|---|---|
| [0001](0001-use-go.md) | Use Go as the primary backend language | Accepted |
| [0002](0002-monorepo-microservices.md) | Microservices in a monorepo | Accepted |
| [0003](0003-database-per-service.md) | Database per service with PostgreSQL | Accepted |
| [0004](0004-kafka-for-events.md) | Apache Kafka for asynchronous communication | Accepted |
| [0005](0005-double-entry-ledger.md) | Double-entry ledger as source of truth | Accepted |
| [0006](0006-grpc-internal-rest-external.md) | gRPC internally, REST externally | Accepted |
| [0007](0007-transactional-outbox.md) | Transactional outbox for event publishing | Accepted |
| [0008](0008-orchestrated-sagas.md) | Orchestrated sagas for money movements | Accepted |
| [0009](0009-read-write-separation.md) | Separate read and write databases | Accepted |
| [0010](0010-sharding-and-cells.md) | Shard by customer and deploy in cells | Accepted |
| [0011](0011-stateless-services-autoscaling.md) | Stateless services with horizontal autoscaling | Accepted |
| [0012](0012-caching-strategy.md) | Caching strategy | Accepted |

## Creating a New ADR

1. Copy `0000-template.md` to `NNNN-short-title.md`.
2. Fill it in and set the status to `Proposed`.
3. Open a PR. Once it is merged, change the status to `Accepted`.
4. ADRs are never deleted. To change a decision, write a new ADR that **supersedes** the old one.
