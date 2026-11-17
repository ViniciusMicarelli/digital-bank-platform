.PHONY: help install dev start stop test lint clean format init

help:
	@echo "Digital Bank Platform - Makefile Commands"
	@echo ""
	@echo "Development:"
	@echo "  start        - Start all services"
	@echo "  stop         - Stop all services"
	@echo "  dev          - Run development server"
	@echo ""
	@echo "Testing:"
	@echo "  test         - Run all tests"
	@echo "  test:unit    - Run unit tests"
	@echo "  test:integration - Run integration tests"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint         - Lint all code"
	@echo "  format       - Format code"
	@echo "  check        - Run type checking"
	@echo ""
	@echo "Infrastructure:"
	@echo "  infra-up     - Start infrastructure (Docker Compose)"
	@echo "  infra-down   - Stop infrastructure"
	@echo "  infra-logs   - View infrastructure logs"
	@echo ""
	@echo "Code Generation:"
	@echo "  codegen      - Generate code from contracts"
	@echo ""
	@echo "Database:"
	@echo "  migrate      - Run database migrations"
	@echo "  migrate:up   - Apply migrations to production"
	@echo "  db:reset     - Drop and recreate database"

install:
	@echo "Installing dependencies..."
	npm install
	@for dir in services/*; do if [ -f $$dir/package.json ]; then cd $$dir && npm install && cd ..; fi; done
	@for dir in packages/*; do if [ -f $$dir/package.json ]; then cd $$dir && npm install && cd ..; fi; done

dev:
	@./scripts/run.sh

start:
	@./scripts/run.sh

stop:
	@./scripts/stop.sh

test:
	@./scripts/test.sh

test:unit:
	@npm run test:unit

test:integration:
	@npm run test:integration

lint:
	@./scripts/lint.sh

format:
	@npm run format
	@for dir in services/*; do if [ -f $$dir/package.json ]; then cd $$dir && npm run format && cd ..; fi; done

check:
	@npm run check
	@for dir in services/* packages/*; do if [ -f $$dir/package.json ]; then cd $$dir && npm run check && cd ..; fi; done

init:
	@echo "Initializing project..."
	@./scripts/init-structure.sh
	@./scripts/infra-up.sh
	@./scripts/migrate.sh

codegen:
	@./scripts/codegen.sh

infra-up:
	@./scripts/infra-up.sh

infra-down:
	@docker-compose -f infrastructure/docker-compose.yml down

infra-logs:
	@docker-compose -f infrastructure/docker-compose.yml logs -f
