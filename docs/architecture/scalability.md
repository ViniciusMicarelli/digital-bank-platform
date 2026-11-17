# Scalability

This document details the architecture's approach to scaling to handle tens of millions of customers.

## Capacity Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Customers | 100 M | 3× headroom beyond target |
| Daily Active Users | 30 M | ~8% of customer base |
| Daily Transactions | 10 B | Avg 100 per customer |
| Peak RPS (reads) | 100k | Across all services |
| Peak TPS (writes) | 20k | Financial transactions |
| Concurrency | 10k | Parallel connections |

## Scalability Strategies

### 1. Horizontal Scaling (Scale-Out)

#### Compute
- **Stateless services**: Any instance can handle any request
- **Kubernetes HPA**: Automatically scale based on CPU or request rate
- **KEDA**: Scale based on Kafka consumer lag
- **Replicas**: Minimum 3 per service across 3 availability zones

```yaml
# Kubernetes HPA example
spec:
  minReplicas: 3
  maxReplicas: 50
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Pods
    pods:
      metric:
        name: requests_per_second
      target:
        averageValue: 100
```

#### Database Writes
- **Primary per shard**: One master per logical shard
- **Synchronous standby**: In different AZ for failover
- **PgBouncer**: Connection pooling for high connection counts
- **Read replicas**: For read-heavy workloads

#### Database Reads
- Separate read replicas (`DB_READ_URL`)
- **Read routing**: Primary for writes, replica for reads
- **Query-specific routing**: Some queries on replicas, some on primary
- **Redis cache**: For frequently accessed data

### 2. Data Sharding

#### Sharding Strategy: By Customer

```
customer_id → shard directory
```

- **Logical shards**: 4,096 logical shards (2^12)
- **Physical mapping**: Hash(customer_id) % 16 (4 physical shards for simplicity)
- **Per-shard**: Independent database stack

#### Benefits

- **Isolation**: Failures contained within shard
- **Parallel queries**: Queries can run on multiple shards
- **Linear scaling**: Add more shards to scale
- **Backup independence**: Can backup shards independently

#### Shard Directory Structure

```
shards/
  0/           → Customer IDs: 0-511
  1/           → Customer IDs: 512-1023
  ...
  15/          → Customer IDs: 7680-8191
```

Customer ID encoding:
- Lower 12 bits: Shard number
- Remaining bits: Sequential within shard

#### Access Pattern

```go
func getShardNumber(customerID int) int {
    return customerID >> 12 // shift right by 12 bits
}
```

### 3. Cell-Based Architecture

#### Organization

Divide customers into cells (groups of shards):

```
Cell 0: Shards 0-15
Cell 1: Shards 16-31
...
Cell N: Shards 16N-16N+15
```

#### Benefits

- **Operational independence**: Can operate a cell without touching others
- **Controlled blast radius**: If one cell fails, others continue
- **Gradual scaling**: Can add one cell at a time
- **Blue-green deployments**: Deploy to new cell, switch traffic

#### Deployment Strategy

```
Cell 0: Running in Prod-AZ1
Cell 1: Running in Prod-AZ2
...
Cell N: Running in both AZs
```

### 4. Read Replicas with Smart Routing

#### Primary Database
- All writes go to primary
- Synchronous replication to standby

#### Read Replicas
- Created automatically after successful write
- Can serve reads after replication lag check

#### Routing Logic

```go
// For money-related queries (must be fresh)
if query.usesMoneyData() {
    return primary
}

// For historical data (can be stale)
if query.isHistorical() {
    return random replica
}
```

### 5. Caching Strategy

#### Redis Cluster

**Cache Contents**
- User sessions
- Device tokens
- Recently viewed balances
- Rate limit counters

**Cache Invalidations**
- Write-through: Update cache and DB together
- Write-behind: Update DB, update cache asynchronously
- TTL-based: Cache expires after time

#### Cache Hierarchy

```
Tier 0: Redis (in-memory, fastest)
Tier 1: Database primary (fast, durable)
Tier 2: Database replica (slower, cheaper)
Tier 3: Cold storage (slowest, cheapest)
```

### 6. Message Scaling

#### Kafka

**Partitions**: 64-256 per topic
- Partitioned by account ID (hot accounts have more partitions)
- Each partition can be processed independently

**Consumers**:
- One consumer group per topic
- Each consumer processes one partition
- Scale consumer group by partition count

**Replication**:
- ISR (In-Sync Replicas): 3+ for durability
- Replication factor: 3

### 7. Load Shedding

When system is overloaded, prioritize requests:

```
Priority 0: Money movements (ledger postings)
Priority 1: Transfers and Pix
Priority 2: Card operations
Priority 3: Bill payments
Priority 4: Account operations
Priority 5: Statement generation
Priority 6: Notifications
Priority 7: Analytics
```

### 8. Time-Series Data

For statements and transaction feeds:

**Database**: Use time-based partitioning
```
transactions_2026_01
transactions_2026_02
transactions_2026_03
...
```

**Archiving**: Move data older than 7 years to cold storage

### 9. GPU and Specialized Hardware

For computationally intensive operations:

- **Interest calculations**: Offload to batch jobs
- **Risk scoring**: Use specialized ML hardware
- **Document processing**: GPU for OCR

## Monitoring and Alerting

### Key Metrics

- Request latency (p50, p95, p99)
- Error rates per service
- Database connection pool utilization
- Kafka consumer lag
- Cache hit ratio
- Replication lag

### Alerting Tiers

| Tier | Services | Target | Alert on |
|------|----------|--------|----------|
| Tier 0 | Ledger, Pix, Card Auth, Login | 99.99% | 99.5% SLO burn |
| Tier 1 | Accounts, Payments, Credit, Feed | 99.95% | 99% SLO burn |
| Tier 2 | Notifications, Reports | 99% | 80% SLO burn |

### SLO Protection

- Circuit breakers to stop propagating failures
- Request shedding when thresholds exceeded
- Graceful degradation (return cached data)

## Data Retention

| Data Type | Hot | Warm | Cold | Archive |
|-----------|-----|------|------|---------|
| Transactions | 2 years | 7 years | 15 years | 30 years |
| Logs | 7 days | 90 days | 1 year | Legal retention |
| Statements | 7 years | - | - | Permanent |
| Cards | Active | 1 year | 3 years | - |

## Disaster Recovery

### RPO (Recovery Point Objective)

- **Ledger**: 0 (no data loss acceptable)
- **Financial data**: < 1 second
- **Non-financial**: < 5 minutes

### RTO (Recovery Time Objective)

- **Critical services**: < 5 minutes
- **Important services**: < 30 minutes
- **Nice-to-have**: < 24 hours

### Multi-Region

1. **Active-Passive**: One region handles all traffic
2. **Active-Active by Cell**: Each cell in different region
3. **Global**: Gradual transition to full active-active

## Cost Optimization

### Database Costs

- Right-sized instances for each workload
- Use managed services for operational overhead
- Spot instances for non-critical workloads

### Compute Costs

- Scale to zero for development
- Spot instances for batch jobs
- Reserved instances for steady-state

### Storage Costs

- Tiered storage (hot/warm/cold)
- Compression for logs
- Deduplication for media

## Capacity Planning

### Phased Rollout

| Phase | Customers | New Capacity |
|-------|-----------|--------------|
| 1 | 0 | Dev/Stage only |
| 2 | 100k | 1 shard, 1 cell |
| 3 | 1M | 4 shards, 2 cells |
| 4 | 10M | 16 shards, 4 cells |
| 5 | 50M | 64 shards, 8 cells |
| 6 | 100M | 128 shards, 12 cells |

### Load Testing

- Weekly: Incremental load testing
- Monthly: Full load test at 1× peak
- Quarterly: Load test at 3× peak
- Before major releases: Target-specific testing