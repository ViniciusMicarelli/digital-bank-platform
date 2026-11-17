#!/bin/bash
# Create the complete directory structure for the Digital Bank Platform

set -e

echo "Creating directory structure..."

# Root
mkdir -p .github/workflows
mkdir -p apps/web
mkdir -p apps/mobile/android
mkdir -p apps/mobile/ios

# Contracts
mkdir -p contracts/proto
mkdir -p contracts/openapi
mkdir -p contracts/events

# Docs
mkdir -p docs/architecture
mkdir -p docs/services
mkdir -p docs/api
mkdir -p docs/events
mkdir -p docs/database
mkdir -p docs/decisions
mkdir -p docs/deployment

# Infrastructure
mkdir -p infrastructure/databases
mkdir -p infrastructure/docker
mkdir -p infrastructure/kubernetes
mkdir -p infrastructure/messaging
mkdir -p infrastructure/monitoring
mkdir -p infrastructure/scripts
mkdir -p infrastructure/terraform

# Packages
mkdir -p packages/go-common
mkdir -p packages/go-errors
mkdir -p packages/go-logger
mkdir -p packages/go-messaging
mkdir -p packages/go-observability
mkdir -p packages/go-security

# Scripts
mkdir -p scripts

# Services (all services will have the same hexagonal structure)
mkdir -p services/api-gateway/cmd/server
mkdir -p services/api-gateway/internal/domain
mkdir -p services/api-gateway/internal/application
mkdir -p services/api-gateway/internal/ports
mkdir -p services/api-gateway/internal/adapters/grpc
mkdir -p services/api-gateway/internal/adapters/postgres
mkdir -p services/api-gateway/migrations
mkdir -p services/api-gateway/config

mkdir -p services/identity-service/cmd/server
mkdir -p services/identity-service/internal/domain
mkdir -p services/identity-service/internal/application
mkdir -p services/identity-service/internal/ports
mkdir -p services/identity-service/internal/adapters/grpc
mkdir -p services/identity-service/internal/adapters/postgres
mkdir -p services/identity-service/internal/adapters/kafka
mkdir -p services/identity-service/migrations
mkdir -p services/identity-service/config

mkdir -p services/user-service/cmd/server
mkdir -p services/user-service/internal/domain
mkdir -p services/user-service/internal/application
mkdir -p services/user-service/internal/ports
mkdir -p services/user-service/internal/adapters/grpc
mkdir -p services/user-service/internal/adapters/postgres
mkdir -p services/user-service/internal/adapters/kafka
mkdir -p services/user-service/migrations
mkdir -p services/user-service/config

mkdir -p services/account-service/cmd/server
mkdir -p services/account-service/internal/domain
mkdir -p services/account-service/internal/application
mkdir -p services/account-service/internal/ports
mkdir -p services/account-service/internal/adapters/grpc
mkdir -p services/account-service/internal/adapters/postgres
mkdir -p services/account-service/internal/adapters/kafka
mkdir -p services/account-service/migrations
mkdir -p services/account-service/config

mkdir -p services/ledger-service/cmd/server
mkdir -p services/ledger-service/internal/domain
mkdir -p services/ledger-service/internal/application
mkdir -p services/ledger-service/internal/ports
mkdir -p services/ledger-service/internal/adapters/grpc
mkdir -p services/ledger-service/internal/adapters/postgres
mkdir -p services/ledger-service/internal/adapters/kafka
mkdir -p services/ledger-service/migrations
mkdir -p services/ledger-service/config

mkdir -p services/transaction-service/cmd/server
mkdir -p services/transaction-service/internal/domain
mkdir -p services/transaction-service/internal/application
mkdir -p services/transaction-service/internal/ports
mkdir -p services/transaction-service/internal/adapters/grpc
mkdir -p services/transaction-service/internal/adapters/postgres
mkdir -p services/transaction-service/internal/adapters/kafka
mkdir -p services/transaction-service/migrations
mkdir -p services/transaction-service/config

mkdir -p services/pix-service/cmd/server
mkdir -p services/pix-service/internal/domain
mkdir -p services/pix-service/internal/application
mkdir -p services/pix-service/internal/ports
mkdir -p services/pix-service/internal/adapters/grpc
mkdir -p services/pix-service/internal/adapters/postgres
mkdir -p services/pix-service/internal/adapters/kafka
mkdir -p services/pix-service/migrations
mkdir -p services/pix-service/config

mkdir -p services/payment-service/cmd/server
mkdir -p services/payment-service/internal/domain
mkdir -p services/payment-service/internal/application
mkdir -p services/payment-service/internal/ports
mkdir -p services/payment-service/internal/adapters/grpc
mkdir -p services/payment-service/internal/adapters/postgres
mkdir -p services/payment-service/internal/adapters/kafka
mkdir -p services/payment-service/migrations
mkdir -p services/payment-service/config

mkdir -p services/card-service/cmd/server
mkdir -p services/card-service/internal/domain
mkdir -p services/card-service/internal/application
mkdir -p services/card-service/internal/ports
mkdir -p services/card-service/internal/adapters/grpc
mkdir -p services/card-service/internal/adapters/postgres
mkdir -p services/card-service/internal/adapters/kafka
mkdir -p services/card-service/migrations
mkdir -p services/card-service/config

mkdir -p services/credit-service/cmd/server
mkdir -p services/credit-service/internal/domain
mkdir -p services/credit-service/internal/application
mkdir -p services/credit-service/internal/ports
mkdir -p services/credit-service/internal/adapters/grpc
mkdir -p services/credit-service/internal/adapters/postgres
mkdir -p services/credit-service/internal/adapters/kafka
mkdir -p services/credit-service/migrations
mkdir -p services/credit-service/config

mkdir -p services/statement-service/cmd/server
mkdir -p services/statement-service/internal/domain
mkdir -p services/statement-service/internal/application
mkdir -p services/statement-service/internal/ports
mkdir -p services/statement-service/internal/adapters/grpc
mkdir -p services/statement-service/internal/adapters/postgres
mkdir -p services/statement-service/internal/adapters/kafka
mkdir -p services/statement-service/migrations
mkdir -p services/statement-service/config

mkdir -p services/notification-service/cmd/server
mkdir -p services/notification-service/internal/domain
mkdir -p services/notification-service/internal/application
mkdir -p services/notification-service/internal/ports
mkdir -p services/notification-service/internal/adapters/grpc
mkdir -p services/notification-service/internal/adapters/postgres
mkdir -p services/notification-service/internal/adapters/kafka
mkdir -p services/notification-service/migrations
mkdir -p services/notification-service/config

# Create .gitkeep files in empty directories to prevent git from ignoring them
touch .github/workflows/.gitkeep
touch apps/web/.gitkeep
touch apps/mobile/android/.gitkeep
touch apps/mobile/ios/.gitkeep
touch contracts/proto/.gitkeep
touch contracts/openapi/.gitkeep
touch contracts/events/.gitkeep
touch docs/architecture/.gitkeep
touch docs/services/.gitkeep
touch docs/api/.gitkeep
touch docs/events/.gitkeep
touch docs/database/.gitkeep
touch docs/decisions/.gitkeep
touch docs/deployment/.gitkeep
touch infrastructure/databases/.gitkeep
touch infrastructure/docker/.gitkeep
touch infrastructure/kubernetes/.gitkeep
touch infrastructure/messaging/.gitkeep
touch infrastructure/monitoring/.gitkeep
touch infrastructure/scripts/.gitkeep
touch infrastructure/terraform/.gitkeep
touch packages/go-common/.gitkeep
touch packages/go-errors/.gitkeep
touch packages/go-logger/.gitkeep
touch packages/go-messaging/.gitkeep
touch packages/go-observability/.gitkeep
touch packages/go-security/.gitkeep
touch scripts/.gitkeep

echo "Directory structure created successfully!"
echo ""
echo "Created directories:"
tree -L 3 --dirsfirst -I 'pkg|node_modules|.git'
