BINARY  := lgtm
PKG     := .
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

.PHONY: run
run: ## Run lgtm in the current repository
	go run $(PKG)

.PHONY: install
install: ## Install the binary into $GOBIN
	CGO_ENABLED=0 go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

.PHONY: test
test: ## Run the tests with the race detector
	CGO_ENABLED=1 go test -race -count=1 ./...

.PHONY: cover
cover: ## Run the tests and write coverage.out
	CGO_ENABLED=1 go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: fmt
fmt: ## Format the code
	golangci-lint fmt

.PHONY: tidy
tidy: ## Tidy go.mod
	go mod tidy

.PHONY: check
check: tidy lint test ## Everything CI checks
	git diff --exit-code go.mod go.sum

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin dist coverage.out
