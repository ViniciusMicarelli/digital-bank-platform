# Every module listed in go.work (contracts, packages and services).
MODULES := $(shell go list -m -f '{{.Dir}}/...')
BIN     := $(CURDIR)/bin
export PATH := $(BIN):$(PATH)

BUF_VERSION          := v1.75.0
PROTOC_GEN_GO        := v1.36.12
PROTOC_GEN_GO_GRPC   := v1.6.2

.PHONY: help certs rotate-certs k8s-validate tf-validate infra-tools up up-obs demo web web-test web-e2e mobile mobile-test mobile-e2e down test test-unit e2e cover lint fmt tidy generate tools run-%

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

web: ## Run the web app in dev mode on http://localhost:5173 (needs `make up`)
	cd apps/web && npm install && npm run dev

web-test: ## Lint, typecheck, unit-test and build the web app
	cd apps/web && npm run lint && npm run typecheck && npm test && npm run build

web-e2e: ## Browser tests (Playwright) against the web image on :8081 (needs `make up`)
	cd apps/web && BASE_URL=http://127.0.0.1:8081 npx playwright test

mobile: ## Run the mobile app (Expo); open it with Expo Go on a phone on the same Wi-Fi (needs `make up`)
	cd apps/mobile && npm install && npx expo start

mobile-test: ## Lint, typecheck and check the mobile app
	cd apps/mobile && npx expo lint && npm run typecheck && npx expo-doctor

mobile-e2e: ## Browser tests of the mobile app's web build on a phone viewport (needs `make up`)
	cd apps/mobile && EXPO_PUBLIC_API_URL=http://localhost:8000 npx expo export --platform web && npx playwright test

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

k8s-validate: ## Lint and render the Helm chart, validate it against Kubernetes schemas
	helm lint infrastructure/kubernetes/digital-bank
	helm template db infrastructure/kubernetes/digital-bank --namespace bank \
		| kubeconform -strict -summary -kubernetes-version 1.33.0 -skip Certificate

tf-validate: ## terraform fmt + validate (no backend, no AWS credentials needed)
	cd infrastructure/terraform && terraform fmt -recursive -check && \
		terraform init -backend=false -input=false >/dev/null && terraform validate

tools: ## Install buf and the protoc plugins into ./bin
	GOBIN=$(BIN) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	GOBIN=$(BIN) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO)
	GOBIN=$(BIN) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC)

infra-tools: ## Install helm and kubeconform into ./bin
	GOBIN=$(BIN) go install helm.sh/helm/v3/cmd/helm@v3.22.1
	GOBIN=$(BIN) go install github.com/yannh/kubeconform/cmd/kubeconform@v0.8.0

generate: ## Regenerate Go code from contracts/proto
	cd contracts && rm -rf gen && buf lint && buf generate

run-%: ## Run a service locally, e.g. `make run-ledger`
	go run ./services/$*-service/cmd/server
