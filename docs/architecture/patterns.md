# Patterns

This document describes the architectural patterns used throughout the platform.

## Hexagonal Architecture (Ports and Adapters)

All services follow the hexagonal (ports and adapters) architecture pattern:

```
                    ┌─────────────────────┐
                    │   Application Layer │
                    │ (Use Cases/Handlers)│
                    └─────────────────────┘
                   /                      \
                  /                        \
         ┌──────────────┐         ┌──────────────┐
         │   Ports      │         │   Ports      │
         │ (Interfaces) │         │ (Interfaces) │
         └──────────────┘         └──────────────┘
            |  (Adapter)            |  (Adapter)
            |                      |
    ┌──────────────┐       ┌──────────────┐
    │   Adapters   │       │   Adapters   │
    │  Inbound    │       │  Outbound    │
    └──────────────┘       └──────────────┘
         |                      |
    ┌──────────────┐       ┌──────────────┐
    │  gRPC        │       │  PostgreSQL  │
    │  Kafka       │       │  Redis       │
    └──────────────┘       └──────────────┘
         |                      |
    ┌──────────────┐       ┌──────────────┐
    │   Domain     │       │   Domain     │
    │  Models     │       │  Models     │
    └──────────────┘       └──────────────┘
```

### Benefits

- **Clear separation of concerns**: Domain logic is independent of infrastructure
- **Easy testing**: Can test domain logic without infrastructure
- **Swappable infrastructure**: Can change databases, message brokers, etc. without changing domain logic
- **Testability**: Ports (interfaces) can be mocked in tests

### Implementation Details

**Domain Layer**
- Contains business entities, value objects, and domain rules
- No dependencies on external systems
- Pure business logic

**Application Layer**
- Orchestrates domain logic
- Handles commands and queries
- Coordinates between domain entities and infrastructure

**Ports (Interfaces)**
- Define contracts for external interactions
- No implementation, just interfaces
- Example: `MoneyRepository`, `KafkaPublisher`

**Adapters (Implementations)**
- Implement ports using concrete technologies
- Example: `PostgresMoneyRepository` implements `MoneyRepository`

### Example Structure

```go
// ports
type MoneyRepository interface {
    GetMoney(ctx context.Context, id string) (*Money, error)
}

// adapters/ports/postgres
type PostgresMoneyRepository struct{}

func (r *PostgresMoneyRepository) GetMoney(ctx context.Context, id string) (*Money, error) {
    // Implementation using PostgreSQL
}
```

## Event-Driven Architecture

Services communicate asynchronously through domain events published to Kafka.

### Event Schema

- Events are defined in `contracts/events/`
- Schemas are versioned
- JSON Schema format for compatibility

### Event Handling

- Consumers are idempotent (can handle the same event multiple times safely)
- Out-of-order events are handled gracefully
- Dead letter queues for failed processing

### Lifecycle

1. Event is published (via transactional outbox)
2. Event is committed to Kafka
3. Consumers poll for new events
4. Processing happens (idempotently)
4. Side effects (notifications, projections, etc.)

## CQRS (Command Query Responsibility Segregation)

Read and write operations are separated into different models.

### Write Model

- The domain model used for writing
- Normalized for consistency
- Stored in the primary database

### Read Model

- Optimized for specific queries
- Can be denormalized
- Stored in optimized format (can be different database)

### Example: Statement Service

**Write Model**: Transaction records (normalized, linked to accounts)

**Read Model**: Pre-computed projection per account (denormalized, ready for fast reads)

**Commands**: `CreateTransaction` → updates write model → publishes event → triggers read model update

**Queries**: `GetAccountStatement` → reads from read model directly (no join to write model)

## API Gateway Pattern

Single entry point for all external client traffic.

### Responsibilities

- Request routing
- Authentication and authorization
- Rate limiting
- Request validation
- Response transformation

### Flow

```
Client → API Gateway → Service Router → Specific Service
```

### Benefits

- Single point of entry for clients
- Centralized security and monitoring
- Service decoupling from clients
- Easy to add new services without client changes

## Saga Pattern for Distributed Transactions

Since we don't use distributed transactions (2-phase commit), we use Sagas.

### Choreography Saga

Services publish events, other services subscribe and react.

### Orchestration Saga

A central orchestrator coordinates the saga steps.

### Example: Fund Transfer Saga (Orchestrated)

1. Client sends `TransferFunds` command
2. Orchestrator checks account balance
3. Orchestator debits sender account (transaction 1)
4. Orchestrator credits recipient account (transaction 2)
5. If any step fails, compensating transactions are executed

### Compensating Transactions

Each transaction in the saga has a corresponding compensation:

| Operation | Compensation |
|-----------|--------------|
| Debit account | Credit account |
| Credit account | Debit account |
| Issue card | Cancel card |
| Create bill payment | Cancel payment |

## Double-Entry Ledger

The core financial system is a double-entry ledger.

### Basic Principle

For every transaction, there must be:
- At least one debit
- At least one credit
- Total debits = Total credits

### Example

```
Account A (Debit): +1000
Account B (Credit): -1000

Total debits: 1000
Total credits: 1000
✓ Balanced
```

### Benefits

- Self-checking: The system can't be inconsistent
- Audit trail: Every movement is recorded
- Accurate balances: Sum of all movements

## Repository Pattern

Each service has its own database. Instead of calling other services, repositories communicate with databases.

### Anti-Corruption Layer

When integrating with external systems (or other services), use an anti-corruption layer to translate between incompatible data formats.

## Circuit Breaker Pattern

For resilience in distributed systems.

### States

1. **Closed**: Normal operation, requests go through
2. **Open**: Too many failures, requests fail immediately
3. **Half-Open**: Testing if service is recovered

### Implementation

- Track failure rate over time window
- Open circuit when threshold exceeded
- Try half-open requests after timeout