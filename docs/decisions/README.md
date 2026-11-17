# Architecture Decision Records (ADRs)

Each ADR records one significant decision: the context, the decision, the alternatives and the consequences. Use [0000-template.md](0000-template.md) for new ones.

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-use-go.md) | Go as the primary backend language | Accepted |
| [0002](0002-monorepo-microservices.md) | Microservices in a monorepo (`go.work`, one module per service) | Accepted |
| [0003](0003-database-per-service.md) | Database per service with PostgreSQL | Accepted |
| [0004](0004-kafka-for-events.md) | Apache Kafka for asynchronous communication | Accepted |
| [0005](0005-double-entry-ledger.md) | Double-entry ledger as the source of truth | Accepted |
| [0006](0006-grpc-internal-rest-external.md) | gRPC internally, REST externally | Accepted |
| [0007](0007-transactional-outbox.md) | Transactional outbox for event publishing | Accepted |
| [0008](0008-orchestrated-sagas.md) | Orchestrated sagas for money movements | Accepted |
| [0009](0009-read-write-separation.md) | Separate read and write models (CQRS) | Accepted |
| [0010](0010-sharding-and-cells.md) | Shard by customer and deploy in cells | Accepted (not implemented yet) |
| [0011](0011-stateless-services-autoscaling.md) | Stateless services with horizontal autoscaling | Accepted |
| [0012](0012-caching-strategy.md) | Caching strategy | Accepted (not implemented yet) |
| [0013](0013-observability.md) | Observability: logs, metrics, traces | Accepted |
| [0014](0014-zero-trust-security.md) | Zero-trust security | Accepted (mTLS not implemented yet) |
| [0015](0015-encryption.md) | Encryption strategy | Accepted (not implemented yet) |
| [0016](0016-idempotency.md) | Idempotency keys with request fingerprints | Accepted |

## Lifecycle

`Proposed` → `Accepted` → (`Deprecated` | `Superseded by ADR-NNNN`). Accepted ADRs are not edited; a change of mind is a new ADR that supersedes the old one.
