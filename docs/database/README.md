# Database Documentation

This directory contains database architecture and design for the Digital Bank Platform.

## Overview

The database architecture follows a **per-service database** pattern with PostgreSQL clusters. Each microservice owns its database with no cross-service direct access.

## Architecture

### Write-First CQRS

Each service maintains:
- **Primary database**: For write operations and source of truth
- **Read replicas**: For read-only queries (after warm-up)
- **Cache layer**: For frequently accessed data (Redis)

```
    App
     │
     │ (writes)
     ▼
  Primary (PostgreSQL) ←─── Outbox (events)
     │
     │ (syncs)
     ▼
  Replica (PostgreSQL) →─── Cache (Redis)
     │                       │
     │                       │
  (reads)              Consumes events
     │                    └───► Updates read model
```

## Database Per Service

| Service | Database | Purpose |
|--------|----------|---------|
| `api-gateway` | `gateway` | Sessions, rate limits, API keys |
| `identity-service` | `identity` | Users, sessions, MFA, devices |
| `user-service` | `users` | Customer profiles, KYC data |
| `account-service` | `accounts` | Checking account metadata |
| `ledger-service` | `ledgers` | Ledger entries, customer accounts, hot accounts |
| `transaction-service` | `transactions` | Transfer records, pending items |
| `pix-service` | `pix` | Pix keys, dynamic QR codes |
| `payment-service` | `payments` | Bill payments, scheduled payments |
| `card-service` | `cards` | Card metadata, PINs, controls |
| `credit-service` | `credit` | Credit limits, invoices, installments |
| `statement-service` | `statements` | Statement projections (read model) |
| `notification-service` | `notifications` | Notification preferences, queues |

## Table Naming Convention

```
{Domain}Table

Examples:
- users_customers
- accounts_checking
- ledgers_entries
- transactions_transfers
- pix_keys
- cards_cards
```

## Table Structure Patterns

### Entity Tables (Primary)

```sql
CREATE TABLE IF NOT EXISTS users_customers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id CHAR(20) NOT NULL UNIQUE,
  name VARCHAR(255) NOT NULL,
  document_type VARCHAR(20) NOT NULL,
  document_number VARCHAR(30) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  cell_id VARCHAR(50),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT fk_cell FOREIGN KEY (cell_id) REFERENCES cells(id)
);
```

### Journal Tables (Audit Trail)

```sql
CREATE TABLE IF NOT EXISTS users_journal (
  id BIGSERIAL PRIMARY KEY,
  table_name VARCHAR(50) NOT NULL,
  record_id CHAR(36) NOT NULL,
  column_name VARCHAR(50),
  old_value TEXT,
  new_value TEXT,
  reason VARCHAR(100),
  performed_by VARCHAR(50),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  cell_id VARCHAR(50)
);
CREATE INDEX idx_journal_table_record ON users_journal(table_name, record_id);
```

### Ledger Tables

```sql
-- Customer accounts (write source)
CREATE TABLE IF NOT EXISTS ledgers_accounts_customers (
  customer_id CHAR(20) PRIMARY KEY,
  account_balance NUMERIC(18,2) NOT NULL DEFAULT 0,
  currency_code CHAR(3) NOT NULL DEFAULT 'BRL',
  cell_id VARCHAR(50)
);

-- Ledger entries (append-only, immutable)
CREATE TABLE IF NOT EXISTS ledgers_entries (
  id BIGSERIAL PRIMARY KEY,
  counterparty_customer_id CHAR(20) NOT NULL,
  counterparty_account_no VARCHAR(50),
  counterparty_account_id VARCHAR(50),
  transaction_id VARCHAR(50) NOT NULL,
  credit_ledger VARCHAR(100),
  debit_ledger VARCHAR(100),
  currency_code CHAR(3) NOT NULL,
  amount NUMERIC(18,2) NOT NULL,
  description VARCHAR(255),
  posted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  cell_id VARCHAR(50)
);
```

See [ledger-service documentation](../services/ledger-service.md) for full table definitions.

## Partitioning Strategy

### Time-Based Partitioning (Monthly)

Large tables are partitioned by month:

```sql
CREATE TABLE IF NOT EXISTS ledgers_entries_2025_01 (
  -- Same schema as main table
  CONSTRAINT pk_ledger PRIMARY KEY (id)
) PARTITION BY RANGE (posted_at) (
  RANGE (POSTED_AT FROM '2025-01-01' TO '2025-02-01')
);
```

Monthly partitions are:
- Dropped after 2 years (retention policy)
- Archived to cold storage (S3/GCS)

### Customer-Based Partitioning (Logical Sharding)

Customers are hashed into 4,096 logical shards. Data is routed accordingly:

```sql
SELECT modulo(customer_id_hash, 4096) AS shard_id;
```

### Customer-Based Sharding (Physical)

Shards are distributed across cells:

```
Customer → Shard ID → Cell → Database
```

## Index Strategy

### Primary Keys

All tables have PRIMARY KEY constraints with auto-generated IDs:
- PostgreSQL UUIDs for main entities
- BIGSERIAL/SEQ for journal entries
- UPPERCASE table name prefix for clarity

### Secondary Indexes

Created only when:
1. Used in WHERE, JOIN, or ORDER BY clauses
2. Selectivity > 5%
3. Cover index for INCLUDE columns
4. Composite indexes for common query patterns

Example:
```sql
CREATE INDEX IF NOT EXISTS idx_accounts_customer_status
ON accounts_checks (customer_id, status);
```

## Read Replicas

### Configuration

```bash
# Database URL with replica host
DB_URL="postgresql://user:pass@host:5432/db?sslmode=require"
DB_READ_URL="postgresql://user:pass@replica-host:5432/db?sslmode=require"
```

### Routing

- **Writes**: Always routed to primary (connection pool, service config)
- **Reads**: Default to read replicas, fall back to primary during promotion

### Replication Lag

Read queries are rejected if lag exceeds threshold (default 5 seconds):

```sql
SELECT pg_stat_get_replication(lag)
```

## Connection Management

### PgBouncer

Pooling layer between services and databases:

```yaml
# services/identity-service/docker-compose.override.yml
identity-service:
  depends_on:
    - pgbouncer
  environment:
    - DATABASE_URL=postgresql://pgbouncer/db
    - DB_MAX_OPEN_CONNS=25
    - DB_MAX_IDLE_CONNS=10

pgbouncer:
  environment:
    - Databases=identity=host/db
    - PoolMode=transaction
    - DefaultPoolSize=50
    - MaxClientConn lifetime=1h
    - MaxDBClientConn=100
```

### Connection Pool Sizing

| Metric | Formula |
|--------|---------|
| `max_connections` | (CPU cores × 4) × concurrency factor |
| `max_cached_connections` | `max_connections` × 2 |
| `max_db_connections` | 1 per primary per shard |
| `default_pool_size` | `max_connections` / `num_shards` |

## Migration Strategy

### Tools

- [`golang-migrate`](https://github.com/golang-migrate/migrate) for versioning
- Migration files stored in `services/<service>/migrations/`

### Workflow

1. Create migration: `migrate create 001_add_field --ext sql --idempotent`
2. Write schema changes (DDL)
3. Test locally: `make db-up && make migrate`
4. Deploy to dev: `migrate -database https://dev.db -path ./migrations -verbose`
5. Deploy to staging: repeat step 4
6. Deploy to production: repeat step 4

### Rollback

- `migrate -database prod -path ./migrations -down 1`
- Keep migrations for 2 years (can recreate schema)

## Backup Strategy

### Physical Backups

```bash
# Daily full backup with WAL archiving
0 0 * * * /scripts/backup-databases.sh
```

### Point-in-Time Recovery

- WAL archiving to object storage
- RPO (Recovery Point Objective): < 1 hour
- RTO (Recovery Time Objective): < 30 minutes

### Retention

| Backup Type | Retention |
|------------|-----------|
| Daily full | 7 days |
| Incremental | 7 days |
| Monthly full | 12 months |
| Quarterly full | 7 years |

## Monitoring

### Metrics

- `database_connections_usedactive` / `database_connections_usedmax` (target: < 80%)
- `pg_stat_database_tup_inserted` rate (queries/sec)
- `pg_stat_replication_replay_lag` (seconds)
- `pg_locks` count (deadlocks: 0)
- `pg_stat_statements` (slow queries, execution time)

### Alerts

| Metric | Warning | Critical |
|--------|---------|----------|
| Connection utilization | > 70% | > 90% |
| Replication lag | > 10s | > 30s |
| Lock waits | > 1/min | > 5/min |
| Deadlocks | > 0 | Any |

## Security

### Authentication

- SCRAM authentication (PostgreSQL)
- TLS for all connections (server-side certificate)

### Encryption

- All sensitive columns encrypted at rest:
  - customer_name
  - document_number
  - password
  - card_pin
- TDE (Transparent Data Encryption) for full database

### Access Control

- Role-based access control (RBAC)
- Service accounts with least privilege
- Connection pool credentials rotated quarterly

## Disaster Recovery

### Strategy

| Service | Architecture | RTO | RPO |
|--------|--------------|----|----|
| Ledger | 3-way sync + async backup | < 5 min | 0 |
| Transaction | 2-way sync + async backup | < 15 min | < 5 min |
| Identity | 2-way sync + async backup | < 30 min | < 10 min |
| Others | Primary + replica | < 60 min | < 15 min |

### Testing

Quarterly failover drills:
1. Primary instance fails
2. Measure failover time
3. Verify data consistency
4. Document any issues

## Directory Structure

| File | Description |
|------|-------------|
| [README.md](README.md) | This database documentation |
| [schema.md](./schema.md) | Database schema design patterns |
| [migrations.md](./migrations.md) | Migration strategy and tools |
| [backups.md](./backups.md) | Backup and recovery procedures |
| [monitoring.md](./monitoring.md) | Database monitoring and alerting |
| [security.md](./security.md) | Database security practices |
| [ledger.md](./ledger.md) | Ledger database design |
