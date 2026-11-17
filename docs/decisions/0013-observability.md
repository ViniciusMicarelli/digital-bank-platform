# ADR-013: Observability Strategy

**Status:** Accepted  
**Date:** 2026-11-16  
**Author:** Platform Team

## Context

The digital bank platform requires robust observability to ensure:
- Real-time monitoring of financial transactions
- Debugging of payment failures and reconciliation issues
- Compliance with financial regulations (audit trails)
- Fast incident response and resolution

Currently, the architecture documents (see ADR-001, ADR-009) define microservices and eventual consistency, but no comprehensive observability strategy exists.

## Decision

**We will implement structured, traceable observability using OpenTelemetry, distributed tracing, structured logging, and metrics aggregation.**

### Components

1. **OpenTelemetry Instrumentation**
   - Instrument all services with OpenTelemetry SDK
   - Use OpenTelemetry Collector for aggregation
   - Export to central observability platform

2. **Distributed Tracing**
   - Generate unique trace IDs at entry points (API gateway)
   - Propagate trace context across all service boundaries
   - Create spans for each business operation
   - Link related transactions via trace context

3. **Structured Logging**
   - JSON format with consistent keys: timestamp, service, trace_id, level, message
   - Context enrichment: user_id, transaction_id, operation_type
   - Log sampling for high-volume services (1% default)
   - PII masking for sensitive data

4. **Metrics**
   - Request latency (p50, p95, p99)
   - Error rates (by type, service, endpoint)
   - Business metrics (transactions per second, transaction types)
   - Resource utilization (CPU, memory, network)

5. **Compliance Logging**
   - Immutable audit logs for financial transactions
   - Minimum 7-year retention (per regulations)
   - Separate audit log index for fast queries

### Service Instrumentation

Each service must:
- Inject trace context from incoming requests
- Create spans for external service calls
- Wrap all business logic in spans
- Log errors and structured events

### Data Flow

```
Client → API Gateway (assign trace ID)
  ↓ (trace context)
  Service A → Service B → Service C (propagate trace)
  ↓
OpenTelemetry Collector (aggregate traces, logs, metrics)
  ↓
Observability Platform (storage, analysis, alerting)
```

## Consequences

### Pros
- End-to-end traceability across distributed services
- Fast diagnosis of cross-service issues
- Regulatory compliance with audit logs
- Proactive monitoring via metrics
- Standardized observability across all services

### Cons
- Additional CPU overhead (1-3% for tracing)
- Network overhead for distributed tracing
- Increased storage costs for logs/traces
- Learning curve for development team

### Mitigations
- Sampling for high-volume services (reduce trace storage)
- Trace context propagation via headers (low overhead)
- Log aggregation with TTL (retain by compliance requirement)
- Training and documented instrumentation patterns

## Related Decisions

- ADR-001: Architectural Style - Microservices with eventual consistency
- ADR-009: Domain Distribution - Event sourcing and eventual consistency
- ADR-014: Zero-Trust Security - Related for audit log protection
- ADR-021: Log Retention Strategy - Future elaboration

## References

- OpenTelemetry: https://opentelemetry.io/
- Distributed Tracing: https://opentelemetry.io/docs/trace/
- Cloud Native Observability: https://www.cncf.io/