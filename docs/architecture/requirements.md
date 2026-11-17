# Architecture Requirements

This document defines the non-functional requirements for the Digital Bank Platform.

## Availability and Reliability

### Service Availability Targets

| Tier | Services | Target Availability | Measurement Window |
|------|----------|---------------------|-------------------|
| Tier 0 | Ledger, Pix, Card Authorization, Login | 99.99% | Monthly |
| Tier 1 | Accounts, Payments, Credit, Feed | 99.95% | Monthly |
| Tier 2 | Statements, Notifications | 99.90% | Monthly |

### Failover Requirements

- **Automatic failover** for all Tier 0 services
- **Failover time**: < 30 seconds
- **Manual failover** with approval for non-critical services
- **Graceful degradation**: When primary fails, return cached or partial data

### Consistency

- **Ledger**: Strong consistency (linearizable reads)
- **Balance checks**: Strong consistency
- **Money movements**: Eventually consistent (< 5 seconds)
- **Statements**: Eventually consistent

## Performance Targets

### Latency Targets

| Operation | p50 | p95 | p99 | p999 | Budget |
|-----------|-----|-----|-----|------|--------|
| Ledger posting | 5 ms | 15 ms | 30 ms | 50 ms | 50 ms |
| Get balance (primary) | 10 ms | 20 ms | 40 ms | 60 ms | 50 ms |
| Get balance (replica) | 5 ms | 10 ms | 25 ms | 40 ms | 30 ms |
| Internal transfer | 50 ms | 100 ms | 200 ms | 300 ms | 200 ms |
| Pix transfer (accepted) | 100 ms | 150 ms | 300 ms | 500 ms | 300 ms |
| Card authorization | 50 ms | 100 ms | 200 ms | 300 ms | 200 ms |
| Pix end-to-end settlement | 500 ms | 2 s | 5 s | 10 s | 5 s |
| Transaction feed (first page) | 30 ms | 60 ms | 100 ms | 150 ms | 100 ms |
| Card issuance | 200 ms | 500 ms | 1 s | 2 s | 1 s |
| Account creation | 100 ms | 200 ms | 500 ms | 1 s | 500 ms |

### Throughput Targets

| Metric | Target | Peak | Notes |
|--------|--------|------|-------|
| Ledger TPS | 5k | 20k | With sharding |
| Kafka RPS | 50k | 200k | With proper partitioning |
| API Gateway RPS | 100k | 500k | Rate limited to 10k per customer |
| DB Connections | 10k | 50k | With PgBouncer |

### Scalability

- **Linear scaling**: Adding a shard should increase capacity proportionally
- **Cross-shard operations**: Minimize network latency
- **Read amplification**: < 2× for balanced reads

## Security Requirements

### Authentication

- **MFA**: Required for all accounts (2+ factors)
- **Session timeout**: 15 minutes for web, 4 hours for mobile
- **Token storage**: Secure enclave/Keychain for mobile
- **Session binding**: Bind to device and IP range

### Authorization

- **RBAC**: Role-based access control for admin APIs
- **Permission delegation**: Users can delegate limited permissions
- **Least privilege**: Only access required resources

### Data Protection

- **At rest**: AES-256 encryption for all sensitive data
- **In transit**: TLS 1.3 for all external communication
- **mTLS**: Mutual TLS for internal service communication
- **Key management**: HashiCorp Vault or AWS KMS

### PCI DSS

- **Card data**: Encrypted with HSM-backed keys
- **Tokenization**: Never store PAN (Primary Account Number)
- **Logging**: Mask PAN in logs
- **Network**: Card data in isolated network

### PII Protection

- **Encryption**: PII encrypted at rest
- **Access logging**: All PII access logged and monitored
- **Retention**: Only retain as long as legally required
- **Deletion**: Secure deletion when records expire

## Compliance Requirements

### Regulatory

- **GDPR**: Data subject rights (access, deletion, export)
- **KYC**: Know Your Customer verification
- **AML**: Anti-Money Laundering monitoring
- **PCI DSS**: Payment Card Industry compliance
- **Local regulations**: Country-specific requirements

### Audit

- **Transaction logging**: Immutable audit trail for all money movements
- **Retention**: Minimum 7 years for financial records
- **Export**: Structured export for regulators

## Observability Requirements

### Logging

- **Format**: Structured JSON logs
- **Level**: DEBUG (dev), INFO (stage), ERROR (prod)
- **Content**: Correlation IDs, service names, operation types
- **Redaction**: PII and sensitive data removed
- **Retention**: 90 days for INFO, 1 year for ERROR

### Metrics

- **Infrastructure**: CPU, memory, disk, network
- **Application**: Request counts, latencies, error rates
- **Business**: Transactions, balances, growth
- **Financial**: Reconciliation, risk indicators

### Tracing

- **OpenTelemetry**: Standardized tracing format
- **Sampling**: 100% for errors, 1% for normal traffic
- **Correlation**: Trace ID through all services
- **Pii**: Redact sensitive data in traces

## Operational Requirements

### Deployment

- **GitOps**: All infrastructure as code in Git
- **CI/CD**: Automated testing and deployment
- **Rollback**: Automatic rollback on failure
- **Canary**: Gradual rollout to 5% of traffic

### Monitoring

- **Dashboards**: Team-specific dashboards
- **Alerting**: PagerDuty integration
- **SLOs**: Service Level Objectives tracking
- **Error tracking**: Automatic error classification

### Incident Response

- **Runbooks**: Step-by-step incident procedures
- **Communication**: Automated incident notifications
- **Post-mortems**: Blameless incident reviews
- **Prevention**: Track and fix root causes

## Development Requirements

### Code Quality

- **Linting**: Automated linting on every commit
- **Testing**: Minimum 80% coverage for core services
- **Reviews**: At least 2 reviewers for PRs
- **Documentation**: API docs, service docs, ADRs

### Testing Strategy

| Test Type | Tools | Coverage Target |
|-----------|-------|-----------------|
| Unit tests | Go testing | 100% for domain |
| Integration tests | Testcontainers | 100% for adapters |
| Contract tests | Pact | 100% for APIs |
| Load tests | k6 | 1× and 3× peak |
| Chaos tests | Chaos Mesh | Resilience |

### Code Generation

- **Protobuf**: `buf generate` for all proto files
- **SQL**: `sqlc generate` for all SQL files
- **OpenAPI**: `openapi-generator` for client SDKs
- **Event schemas**: `jsonschema2proto` for event contracts

## Infrastructure Requirements

### Infrastructure as Code

- **Provider**: Terraform
- **State**: Remote backend (S3 + DynamoDB locking)
- **Modules**: Reusable Terraform modules
- **Drift detection**: Automatic detection and alerting

### Containers

- **Base**: Alpine or Distroless images
- **Multistage**: Separate build and runtime stages
- **Scanning**: Automatic vulnerability scanning
- **Signing**: Cosign for image signing

### Networking

- **Service mesh**: Istio or Linkerd
- **mTLS**: Automatic mutual TLS
- **Network policies**: Default deny, explicit allow
- **VPC**: Isolated VPC per environment

## Cost Requirements

### Optimization

- **Spot instances**: For non-critical workloads
- **Auto-scaling**: Right-size resources automatically
- **Right-sizing**: Regular resource utilization reviews
- **Reserved capacity**: For steady-state workloads

### Budgets

- **Per-service budgets**: Cost tracking per service
- **Alert thresholds**: Warn at 80%, critical at 90%
- **Monthly reviews**: Monthly cost optimization meetings

## Disaster Recovery

### RTO/RPO

| Service Class | RTO | RPO | Strategy |
|---------------|-----|-----|----------|
| Ledger | 1 min | 0 | Synchronous replication, hot standby |
| Pix | 5 min | 5 min | Asynchronous replication, warm standby |
| Cards | 15 min | 1 min | Asynchronous replication |
| Accounts | 30 min | 5 min | Asynchronous replication |
| Non-critical | 1 hour | 1 hour | Backup to cold storage |

### Backup

- **Ledger**: Every 5 minutes, stored in 3 regions
- **Accounts**: Every 15 minutes, stored in 2 regions
- **Logs**: Every hour, stored for 90 days
- **Point-in-time recovery**: Every 5 minutes for ledger

## Technical Constraints

### Technology Choices

- **Language**: Go 1.23+ (memory efficiency, performance)
- **Database**: PostgreSQL 17 (ACID compliance, ecosystem)
- **Message broker**: Kafka (throughput, durability)
- **Cache**: Redis (speed, ecosystem)
- **Kubernetes**: Managed Kubernetes (EKS, GKE, AKS)

### Version Compatibility

- **Go versions**: Support 2 consecutive LTS versions
- **PostgreSQL**: Upgrade semver major versions annually
- **Protobuf**: Upgrade to major version at release time
- **Kafka**: Rolling upgrades, no downtime

### Platform Constraints

- **Multi-cloud**: Not required initially, architecture supports it
- **K8s versions**: Support 2 consecutive minor versions
- **Cloud providers**: Provider-agnostic where possible

## Acceptance Criteria

### Non-Goals (Things We Will Not Do Initially)

- ❌ Blockchain-based ledgers (overkill for our needs)
- ❌ Sharding at startup (can shard later)
- ❌ Multi-region replication initially
- ❌ Serverless for core services (latency concerns)
- ❌ NoSQL for money (need ACID guarantees)

### Future Considerations (May Add Later)

- ✓ Multi-region active-active
- ✓ Automatic sharding
- ✓ Serverless compute for burst traffic
- ✓ GraphQL for flexible querying
- ✓ Edge computing for lower latency