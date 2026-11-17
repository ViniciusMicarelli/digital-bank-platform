# ADR Index

This document provides an overview of all Architecture Decision Records (ADRs) for the Digital Bank Platform.

## Status Summary

| ADR | Title | Status | Date | Notes |
|-----|-------|--------|------|-------|
| 0001 | Domain-Driven Design (DDD) | ✅ Accepted | 2024-01-15 | Foundation for domain model |
| 0002 | Authentication Strategy | ✅ Accepted | 2024-02-20 | Identity, sessions, OAuth 2.0 |
| 0003 | Event Sourcing | ✅ Accepted | 2024-03-10 | Immutable event log |
| 0004 | API Design Principles | ✅ Accepted | 2024-04-05 | REST + gRPC, versioning |
| 0005 | Ledger Service Design | ✅ Accepted | 0005 | Double-entry accounting |
| 0006 | User Service Design | ✅ Accepted | 0006 | User lifecycle management |
| 0007 | Transaction Service Design | ✅ Accepted | 0007 | Payment processing |
| 0008 | Wallet Service Design | ✅ Accepted | 0008 | Wallet management |
| 0009 | Notification Service Design | ✅ Accepted | 0009 | Event notifications |
| 0010 | API Gateway Design | ✅ Accepted | 0010 | Request routing and auth |
| 0011 | Database Selection | ✅ Accepted | 0011 | PostgreSQL + DynamoDB |
| 0012 | API Gateway Load Balancing | ✅ Accepted | 0012 | Regional load balancing |
| 0013 | Observability Strategy | ✅ Accepted | 0013 | Tracing, logging, metrics |
| 0014 | Zero-Trust Security Architecture | ✅ Accepted | 0014 | Defense in depth |
| 0015 | Encryption Strategy | ✅ Accepted | 0015 | Multi-layer encryption |
| 0016 | Mutual TLS (mTLS) | 🚧 Draft | 0016 | Internal service authentication |
| 0017 | CI/CD Pipeline Design | 🚧 Draft | 0017 | Build, test, deploy |
| 0018 | Disaster Recovery Strategy | ❌ Superseded | 0018 | Superseded by ADR-019 |
| 0019 | High Availability Strategy | ✅ Accepted | 0019 | Multi-region active-active |
| 0020 | Feature Flag Strategy | ❌ Superseded | 0020 | Superseded by ADR-021 |
| 0021 | Feature Flag Management | ✅ Accepted | 0021 | Flagd-based feature flags |
| 0022 | Caching Strategy | 🚧 Draft | 0022 | Redis + read-through caching |
| 0023 | Key Management Strategy | 🚧 Draft | 0023 | Cloud KMS + dedicated KMS |
| 0024 | Tokenization Service | ❌ Superseded | 0024 | Superseded by ADR-025 |
| 0025 | Tokenization Strategy | ✅ Accepted | 0025 | Payment tokenization |
| 0026 | Audit Logging | ❌ Superseded | 0026 | Superseded by ADR-027 |
| 0027 | Compliance and Audit Logging | ✅ Accepted | 0027 | Regulatory audit trails |
| 0028 | Health Check Strategy | ✅ Accepted | 0028 | Service health monitoring |

## Status Definitions

- ✅ **Accepted** - Decision has been reviewed and approved
- 🚧 **Draft** - Decision is under discussion and may change
- ❌ **Superseded** - Decision has been replaced by a newer ADR
- ⏳ **Deprecated** - Decision is no longer used but may still be in production

## Status by Category

### Architecture
- [x] Domain-Driven Design (DDD)
- [x] Microservices Architecture
- [x] Event Sourcing
- [x] High Availability Strategy
- [x] API Gateway Design
- [x] Database Selection

### Security
- [x] Zero-Trust Security Architecture
- [x] Encryption Strategy
- [x] Mutual TLS (mTLS)
- [x] Tokenization Strategy
- [x] API Access Control

### Operations
- [x] Observability Strategy
- [x] API Gateway Load Balancing
- [x] CI/CD Pipeline Design
- [x] Disaster Recovery Strategy
- [x] Health Check Strategy

### Development
- [x] Authentication Strategy
- [x] API Design Principles
- [x] Service Design (all services)
- [x] Caching Strategy
- [x] Feature Flag Management

### Compliance
- [x] Compliance and Audit Logging
- [x] Key Management Strategy
- [x] PCI-DSS Compliance
- [x] GDPR Compliance

## Navigation by Topic

### Domain Services
- [User Service](0006-user-service.md) - User lifecycle and profiles
- [Wallet Service](0008-wallet-service.md) - Wallet management
- [Transaction Service](0007-transaction-service.md) - Payment processing
- [Ledger Service](0005-ledger-service.md) - Accounting engine

### Infrastructure
- [API Gateway](0010-api-gateway.md) - Request routing and management
- [API Gateway Load Balancing](0012-api-gateway-load-balancing.md) - Regional load balancing
- [Database Selection](0011-database-selection.md) - Database technologies
- [Observability Strategy](0013-observability.md) - Tracing, logging, metrics
- [Health Check Strategy](0028-health-check.md) - Service health monitoring

### Security
- [Zero-Trust Security Architecture](0014-zero-trust-security.md) - Security principles
- [Encryption Strategy](0015-encryption.md) - Data protection
- [Mutual TLS (mTLS)](0016-mutual-tls.md) - Inter-service authentication
- [Tokenization Strategy](0025-tokenization.md) - Card data protection
- [API Access Control](0025-api-access-control.md) - Authorization

### Operations
- [CI/CD Pipeline Design](0017-ci-cd.md) - Continuous deployment
- [Disaster Recovery Strategy](0019-disaster-recovery.md) - DR planning
- [Feature Flag Strategy](0021-feature-flags.md) - Feature toggle management

## Navigation by Status

### Accepted (15)
- [ADR-001: Domain-Driven Design](0001-adr.md)
- [ADR-002: Authentication Strategy](0002-authentication.md)
- [ADR-003: Event Sourcing](0003-event-sourcing.md)
- [ADR-004: API Design Principles](0004-api-design-principles.md)
- [ADR-005: Ledger Service Design](0005-ledger-service.md)
- [ADR-006: User Service Design](0006-user-service.md)
- [ADR-007: Transaction Service Design](0007-transaction-service.md)
- [ADR-008: Wallet Service Design](0008-wallet-service.md)
- [ADR-009: Notification Service Design](0009-notification-service.md)
- [ADR-010: API Gateway Design](0010-api-gateway.md)
- [ADR-011: Database Selection](0011-database-selection.md)
- [ADR-012: API Gateway Load Balancing](0012-api-gateway-load-balancing.md)
- [ADR-013: Observability Strategy](0013-observability.md)
- [ADR-014: Zero-Trust Security Architecture](0014-zero-trust-security.md)
- [ADR-015: Encryption Strategy](0015-encryption.md)
- [ADR-019: High Availability Strategy](0019-high-availability.md)
- [ADR-021: Feature Flag Management](0021-feature-flags.md)
- [ADR-025: Tokenization Strategy](0025-tokenization.md)
- [ADR-027: Compliance and Audit Logging](0027-compliance-audit.md)
- [ADR-028: Health Check Strategy](0028-health-check.md)

### Draft (4)
- [ADR-016: Mutual TLS (mTLS)](0016-mutual-tls.md)
- [ADR-017: CI/CD Pipeline Design](0017-ci-cd.md)
- [ADR-022: Caching Strategy](0022-caching.md)
- [ADR-023: Key Management Strategy](0023-key-management.md)

### Superseded (4)
- [ADR-018: Disaster Recovery Strategy](0018-disaster-recovery.md)
- [ADR-020: Feature Flag Strategy](0020-feature-flags.md)
- [ADR-024: Tokenization Service](0024-tokenization.md)
- [ADR-026: Audit Logging](0026-audit-logging.md)

## Decision History

### Phase 1: Foundation (2024-01-00 to 2024-03-15)
- ADR-001 to ADR-004: Core architectural decisions

### Phase 2: Domain Services (2024-03-16 to 2024-05-01)
- ADR-005 to ADR-011: Service-level decisions and technologies

### Phase 3: Infrastructure (2024-05-02 to 2024-06-01)
- ADR-012 to ADR-014: Gateway, observability, and security

### Phase 4: Advanced Security (2024-06-02 to 2024-07-01)
- ADR-015 to ADR-016: Encryption, mTLS, and tokenization

### Phase 5: Operational Excellence (2024-07-02 to 2024-08-01)
- ADR-017 to ADR-021: CI/CD, caching, feature flags, HA, and compliance

## Open Decisions

The following topics still require decisions:

### High Priority
1. **Caching Strategy (ADR-022)** - Cache invalidation and consistency model
2. **Key Management Strategy (ADR-023)** - Cloud vs dedicated KMS
3. **Mutual TLS (ADR-016)** - mTLS implementation approach

### Medium Priority
1. **API Access Control (ADR-025)** - RBAC vs ABAC
2. **Compliance and Audit Logging (ADR-027)** - Audit log format and retention

## Maintaining the Index

This document is automatically maintained by the ADR management tool. When creating a new ADR:
1. Add your ADR to the appropriate section in this document
2. Update the status summary table
3. Update the navigation by topic and status

## ADR Template

When creating a new ADR, use the following template:

```markdown
# ADR-N: Title

**Status:** {proposed | accepted | deprecated | superseded}  
**Date:** YYYY-MM-DD  
**Author:** {your name}

## Context

Describe the context and problem you are trying to solve. Include:
- What is the problem?
- What are we trying to achieve?
- What constraints do we have?
- What alternatives have we considered?

## Decision

Describe the decision and the solution chosen. Be specific about what you are deciding.

## Consequences

What are the consequences of this decision? Include:
- **Pros:** What are the advantages?
- **Cons:** What are the disadvantages?
- **Mitigations:** How do we mitigate the disadvantages?

## Related Decisions

Reference other ADRs that are related to this decision.

## References

- Links to relevant documentation and standards
- Links to related issues or PRs
```

## Contribution Guidelines

1. **Title:** Clear and descriptive summary of the decision
2. **Context:** Be detailed about the problem, constraints, and alternatives considered
3. **Decision:** Be specific about what decision is being made
4. **Consequences:** Be explicit about the implications
5. **Review:** Have the decision reviewed by the architecture review board

## Tools

- Use the ADR management tool to create, edit, and list ADRs
- See the [ADR Index](0000-index.md) for the complete list of all ADRs