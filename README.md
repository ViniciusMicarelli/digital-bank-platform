# Digital Bank Platform

A microservices-based digital banking platform built with Go and gRPC for high-performance financial transactions.

## Table of Contents

- [Project Overview](#project-overview)
- [Tech Stack](#tech-stack)
- [Project Structure](#project-structure)
- [Quick Start](#quick-start)
- [Service Dependencies](#service-dependencies)
- [Build & Deploy](#build-deploy)
- [CI/CD Pipeline](#cicd-pipeline)
- [Documentation](#documentation)

## Project Overview

The Digital Bank Platform is a distributed system designed for secure, scalable, and compliant financial operations. It supports core banking functions including account management, payments, transfers, card issuance, and authentication.

### Key Features

- **Accounts**: Multi-currency account creation and management
- **Ledger**: Double-entry bookkeeping for transaction processing
- **Payments**: Credit/debit card payment processing with PCI compliance
- **Cards**: Virtual and physical card lifecycle management
- **Transfers**: SEPA and instant payment transfers
- **Auth**: OAuth2/OIDC authentication and authorization
- **Users**: User profiles and preferences management

### Security Highlights

- **mTLS** between all internal services
- **TLS 1.3** for external API communication
- **PCI-DSS Level 1** compliant for payment processing
- **LGPD/GDPR** compliant for data handling
- **AES-256** encryption for sensitive data at rest
- **Field-level encryption** for PCI-sensitive data
- **Automatic key rotation** via HashiCorp Vault
- **JWT-based authentication** with short-lived tokens

## Tech Stack

| Technology | Purpose |
|------------|---------|
| **Go 1.21** | Core microservice implementation |
| **gRPC** | Internal service-to-service communication |
| **PostgreSQL 15** | Primary database for transactional data |
| **gRPC-Go** | gRPC client and server generation |
| **Kong** | API gateway with mTLS termination |
| **OpenResty** | Rate limiting and request routing |
| **Redis** | Caching and session management |
| **OpenTelemetry** | Distributed tracing and metrics |
| **Prometheus** | Metrics collection and alerting |
| **Jaeger** | Distributed tracing backend |
| **HashiCorp Vault** | Secrets and key management |
| **Vault Agent** | Automatic secrets rotation |
| **Kubernetes** | Container orchestration |
| **Helm** | Kubernetes package manager |

## Project Structure

```
digital-bank-platform/
├── api/
│   ├── auth/                    # OAuth2/OIDC service
│   │   ├── proto/               # gRPC protobuf definitions
│   │   ├── http/                # REST API handlers
│   │   └── cmd/                # Service entrypoint
│   ├── ledger/                 # Transaction ledger service
│   │   ├── proto/               # gRPC protobuf definitions
│   │   ├── cmd/                # Service entrypoint
│   │   └── migrations/         # Database migrations
│   ├── payments/               # Payment processing service
│   │   ├── proto/               # gRPC protobuf definitions
│   │   ├── cmd/                # Service entrypoint
│   │   └── tokenization/       # Card token storage
│   ├── cards/                  # Card lifecycle service
│   │   ├── proto/               # gRPC protobuf definitions
│   │   ├── cmd/                # Service entrypoint
│   │   └── vault/              # Vault integration
│   ├── transfers/              # Transfer service
│   │   ├── proto/               # gRPC protobuf definitions
│   │   ├── cmd/                # Service entrypoint
│   │   └── routing/           # Payment routing logic
│   └── users/                  # User management service
│       ├── proto/               # gRPC protobuf definitions
│       └── cmd/                # Service entrypoint
├── infrastructure/
│   ├── k8s/                    # Kubernetes manifests
│   │   ├── base/              # Base deployments and services
│   │   ├── network/          # Network policies, ingress
│   │   ├── monitoring/       # Prometheus, Jaeger
│   │   └── vault/             # Vault secrets management
│   ├── docker/                # Dockerfiles and Compose
│   └── scripts/              # Deployment scripts
├── pkg/                       # Shared code packages
│   ├── auth/                  # Authentication helpers
│   ├── ledger/                # Ledger implementation
│   ├── payments/              # Payment processing
│   ├── validation/            # Input validation
│   └── vault/                 # Vault client
├── config/                    # Configuration templates
│   ├── environment.yaml
│   └── secrets.yaml
├── scripts/                   # Utility scripts
│   ├── generate-proto.sh     # gRPC proto generation
│   ├── gen-keycloak.sh       # Keycloak setup
│   └── vault-init.sh         # Vault secrets
├── tests/
│   ├── e2e/                   # End-to-end tests
│   ├── integration/          # Integration tests
│   └── unit/                 # Unit tests
└── docs/                      # Documentation
    ├── architecture.md
    └── api/                   # API documentation
```

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Docker Compose V2 (required for mTLS)
- Go 1.21+
- gRPC Tools

### Development Setup

```bash
# Start the local environment
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d

# Wait for services to be ready
sleep 30

# Access the services
# - API Gateway: http://localhost:8000
# - gRPC clients: connect to localhost:50051
```

### Generate gRPC Clients

```bash
# Install protoc and gRPC plugins
brew install protobuf grpcio grpc_tools

# Generate Go client stubs
go generate ./api/...

# Run the generator
protoc --go_out=. \
    --go_opt=paths=source_relative \
    --go-grpc_out=. \
    --go-grpc_opt=paths=source_relative \
    api/auth/proto/*.proto
```

### Run Services Locally

```bash
# Start services in development mode
go run ./api/auth/cmd/server.go &
go run ./api/ledger/cmd/server.go &
go run ./api/payments/cmd/server.go &
go run ./api/cards/cmd/server.go &
go run ./api/transfers/cmd/server.go &
go run ./api/users/cmd/server.go &
```

## Service Dependencies

```
┌────────────────────────────────────────────────────────────┐
│                        API Gateway                         │
│                   (Kong + OpenResty)                       │
└────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌─────────────────────┐
                    │         mTLS        │
                    └─────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
              ▼               ▼               ▼
    ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
    │   Auth       │  │   Ledger     │  │   Payments    │
    │   Service    │  │   Service    │  │   Service     │
    └──────┬───────┘  └──────┬───────┘  └──────┬───────┘
           │                 │                 │
           │                 │                 │
           ▼                 ▼                 ▼
    ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
    │   Accounts   │  │   Ledger     │  │    Cards     │
    │   Service    │  │   DB         │  │   Service    │
    └──────────────┘  │             │  └──────┬───────┘
                      └──────┬───────┘        │
                             │               │
                             ▼               ▼
                      ┌──────────────┐  ┌──────────────┐
                      │   Transfer   │  │    Vault     │
                      │   Service    │  │   (Secrets)  │
                      └──────┬───────┘  └──────────────┘
                             │
                             ▼
                      ┌──────────────┐
                      │    Users     │
                      │   Service    │
                      └──────┬───────┘
                             │
                              ▼
                      ┌──────────────┐
                      │     Redis    │
                      │   (Cache)    │
                      └──────────────┘
                             
                              ▼
                      ┌──────────────┐
                      │  PostgreSQL  │
                      │    (Primary) │
                      └──────────────┘

External Services:
┌────────────────────────────────────────────────────────────┐
│                    External Services                       │
├────────────────────────────────────────────────────────────┤
│  Keycloak         │ Identity management                   │
│  OpenTelemetry    │ Distributed tracing                   │
│  Prometheus      │ Metrics collection                    │
│  Jaeger          │ Trace backend                         │
│  Vault           │ Secrets management                     │
│  Stripe          │ Payment processing (optional)          │
└────────────────────────────────────────────────────────────┘
```

## Build & Deploy

### Building Images

```bash
# Build all services
make build

# Build and push to registry
docker build -t digitalbank/ledger:latest -f api/ledger/Dockerfile .
docker push registry.digitalbank.io/ledger:latest
```

### Local Deployment

```bash
# Start with docker-compose
docker compose up -d

# Or with kubectl
kubectl apply -f infrastructure/k8s/base/
```

### Kubernetes Deployment

```bash
# Apply configuration
helm repo add digitalbank https://charts.digitalbank.io
helm install digitalbank-digitalbank digitalbank/digitalbank \
  --create-namespace \
  --namespace digitalbank \
  --values values.yaml

# Verify deployment
kubectl get pods -n digitalbank
```

### Environment Variables

```bash
# .env.local
API_KEY=your-api-key
DATABASE_URL=postgresql://user:pass@localhost:5432/digitalbank
SECRET_KEY=your-secret-key
VAULT_ADDR=http://vault:8200
VAULT_TOKEN=your-vault-token
```

## CI/CD Pipeline

### Pipeline Stages

```mermaid
graph TD
    A[Developer Push] --> B[Lint & Test]
    B --> C[Build Container]
    C --> D[Run E2E Tests]
    D --> E{Push to Environment}
    E -->|Staging| F[Run Integration Tests]
    E -->|Production| G[Run Production Tests]
    F --> H[Manual Approval]
    G --> H
    H --> I[Deploy to Production]
    I --> J[Run Smoke Tests]
```

### GitHub Actions Workflow

```yaml
name: CI/CD Pipeline

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - run: make lint

  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
      redis:
        image: redis:7
    steps:
      - uses: actions/checkout@v3
      - run: make test

  build:
    runs-on: ubuntu-latest
    needs: lint
    steps:
      - uses: actions/checkout@v3
      - run: make build
      - run: docker push ${REGISTRY}:${BUILD_REF}

  deploy-staging:
    needs: build
    runs-on: ubuntu-latest
    environment: staging
    steps:
      - uses: actions/checkout@v3
      - run: kubectl apply -f k8s/staging/

  deploy-production:
    needs: deploy-staging
    runs-on: ubuntu-latest
    environment: production
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v3
      - run: kubectl apply -f k8s/production/
```

## API Documentation

### gRPC Services

See [/api/](/api/) directory for proto definitions.

### REST APIs

| Endpoint | Method | Description |
|----------|--------|-------------|
| /v1/auth/login | POST | User authentication |
| /v1/auth/token | POST | Token refresh |
| /v1/auth/logout | POST | User logout |
| /v1/accounts | GET | List accounts |
| /v1/accounts/{id} | GET | Get account |
| /v1/accounts | POST | Create account |
| /v1/accounts/{id}/close | POST | Close account |

### WebSocket Events

| Event | Description |
|-------|-------------|
| payment.completed | Payment successfully processed |
| transfer.completed | Transfer successfully processed |
| card.issued | Virtual card created |
| card.expired | Card expiration event |

## License

MIT License - See [LICENSE](LICENSE) file for details.

---

For technical documentation, see [ARCHITECTURE.md](ARCHITECTURE.md).