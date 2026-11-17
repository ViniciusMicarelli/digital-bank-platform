# Documentation

Technical documentation for the Digital Bank Platform. What is built and what is next: [ROADMAP.md](ROADMAP.md).

## Architecture

- [Non-functional requirements](architecture/requirements.md): scale targets, latency SLOs (ms), availability, RPO/RTO
- [Scalability](architecture/scalability.md): horizontal scaling, read/write separation, sharding, cells, caching
- [Overview](architecture/overview.md): system context, containers and main flows
- [Patterns](architecture/patterns.md): hexagonal architecture, sagas, outbox, idempotency, CQRS
- [Security](architecture/security.md): authentication, authorization, encryption and card data
- [Observability](architecture/observability.md): logs, metrics, traces and alerts

## Operations

- [Deployment and operations](deployment.md): Kubernetes chart, Terraform, KMS, certificates, background jobs, runbook

## Services

| Service | Description |
|---|---|
| [api-gateway](services/api-gateway.md) | Edge entry point |
| [identity-service](services/identity-service.md) | Authentication and sessions |
| [user-service](services/user-service.md) | Customer profile and KYC |
| [account-service](services/account-service.md) | Checking accounts |
| [ledger-service](services/ledger-service.md) | Double-entry ledger |
| [transaction-service](services/transaction-service.md) | Money movement orchestration |
| [pix-service](services/pix-service.md) | Pix instant payments |
| [payment-service](services/payment-service.md) | Bills and scheduled payments |
| [card-service](services/card-service.md) | Debit and credit cards |
| [credit-service](services/credit-service.md) | Credit limits and invoices |
| [statement-service](services/statement-service.md) | Statements and history |
| [notification-service](services/notification-service.md) | Push, email and SMS |
| kms-service | Envelope encryption for the other services' keys (see [deployment](deployment.md#keys-kms-service)) |

## Contracts and Data

- [API guidelines](api/README.md): REST and gRPC conventions, errors, pagination, idempotency
- [Event catalog](events/README.md): event envelope, topics and the full catalog
- [Database guidelines](database/README.md): database per service, money, migrations, ledger model

## Decisions

- [Architecture Decision Records](decisions/README.md)

## Glossary

| Term | Meaning |
|---|---|
| **Pix** | Brazilian instant payment system run by the Central Bank (BCB), available 24/7 |
| **DICT** | Pix directory that maps Pix keys to accounts |
| **SPI** | Instant Payment System, the Pix settlement infrastructure |
| **Boleto** | Brazilian bank payment slip |
| **CPF / CNPJ** | Brazilian taxpayer IDs for individuals and companies |
| **KYC** | Know Your Customer, the identity verification process |
| **PAN** | Primary Account Number, the card number |
| **Invoice** | Monthly credit card bill |
| **Saga** | A sequence of local transactions with compensating actions |
| **Outbox** | A pattern for publishing events atomically with database writes |
