# Every module listed in go.work (packages and services).
MODULES := $(shell go list -m -f '{{.Dir}}/...')

.PHONY: help test cover lint fmt tidy

help: ## List available targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-8s %s\n", $$1, $$2}'

test: ## Run all tests with the race detector
	go test -race $(MODULES)

cover: ## Run tests and print coverage per package
	go test -cover $(MODULES)

lint: ## Check formatting and run go vet
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run 'make fmt'"; exit 1; }
	go vet $(MODULES)

fmt: ## Format all Go code
	gofmt -w .

tidy: ## Run go mod tidy in every module
	@for d in $$(go list -m -f '{{.Dir}}'); do (cd $$d && go mod tidy); done
