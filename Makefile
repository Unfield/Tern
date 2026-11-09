.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/Unfield/Tern/internal/version.Version=$(VERSION)

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the tern CLI into ./bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/tern ./cmd/tern

.PHONY: test
test: ## Run Go tests with the race detector
	go test -race -count=1 ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint and the TypeScript typecheck
	golangci-lint run
	pnpm -r typecheck

.PHONY: fmt
fmt: ## Format Go code
	golangci-lint fmt

.PHONY: tidy
tidy: ## Tidy go.mod
	go mod tidy

.PHONY: check
check: vet lint test ## Run everything CI runs
	go mod tidy -diff

.PHONY: clean
clean: ## Remove build output
	rm -rf bin dist
