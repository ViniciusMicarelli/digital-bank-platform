# Every module listed in go.work (contracts, packages and services).
MODULES := $(shell go list -m -f '{{.Dir}}/...')
BIN     := $(CURDIR)/bin
export PATH := $(BIN):$(PATH)

BUF_VERSION          := v1.75.0
PROTOC_GEN_GO        := v1.36.12
PROTOC_GEN_GO_GRPC   := v1.6.2

.PHONY: help certs rotate-certs up up-obs demo down test test-unit e2e cover lint fmt tidy generate tools run-%

help: ## List available targets
	@grep -E '^[a-z%-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

certs: ## Generate the development CA and mTLS certificates (infrastructure/certs)
	go run ./packages/go-platform/cmd/devcerts -out infrastructure/certs

rotate-certs: ## Reissue every workload certificate (running services reload them, no restart)
	go run ./packages/go-platform/cmd/devcerts -out infrastructure/certs -reissue

infrastructure/certs/ca.pem:
	$(MAKE) certs

up: infrastructure/certs/ca.pem ## Build and start the local stack (docker compose)
	docker compose up -d --build --wait

up-obs: infrastructure/certs/ca.pem ## Start the stack plus Grafana (http://127.0.0.1:3000)
	docker compose --profile observability up -d --build --wait

demo: ## Walk through the public API with curl (needs `make up` and jq)
	./scripts/demo.sh

down: ## Stop the local stack
	docker compose --profile observability down

test: ## Run all tests, including integration tests (needs `make up`)
	PGTEST_REQUIRED=1 go test -race -count=1 $(MODULES)

test-unit: ## Run tests; integration tests are skipped when Postgres is down
	go test -race $(MODULES)

# Host-side gRPC clients (e2e tests, grpcurl) use the dev-client identity.
DEV_CLIENT_TLS := TLS_CA=$(CURDIR)/infrastructure/certs/ca.pem \
	TLS_CERT=$(CURDIR)/infrastructure/certs/dev-client.pem \
	TLS_KEY=$(CURDIR)/infrastructure/certs/dev-client-key.pem

e2e: ## Run end-to-end tests against the running stack (`make up` first)
	$(DEV_CLIENT_TLS) go test -tags e2e -count=1 -v ./tests/e2e/...

cover: ## Print coverage per package
	go test -cover $(MODULES)

lint: ## Check formatting, run go vet and buf lint
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run 'make fmt'"; exit 1; }
	go vet $(MODULES)
	@if command -v buf >/dev/null; then cd contracts && buf lint; fi

fmt: ## Format all Go code
	gofmt -w .

tidy: ## Run go mod tidy in every module
	@for d in $$(go list -m -f '{{.Dir}}'); do (cd $$d && GOWORK=off go mod tidy); done

tools: ## Install buf and the protoc plugins into ./bin
	GOBIN=$(BIN) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	GOBIN=$(BIN) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO)
	GOBIN=$(BIN) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC)

generate: ## Regenerate Go code from contracts/proto
	cd contracts && rm -rf gen && buf lint && buf generate

run-%: ## Run a service locally, e.g. `make run-ledger`
	go run ./services/$*-service/cmd/server
