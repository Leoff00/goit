.PHONY: help fmt fmt-check test-example test-unit test-integration test test-race vet build run generate-only test-inline check

FILE ?= examples/sum/main.go
GOFILES = $$(find cmd internal -name '*.go' -type f)

help: ## List available commands
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*## "} {printf "%-16s %s\n", $$1, $$2}'

fmt: ## Format project Go files
	gofmt -w $(GOFILES)

fmt-check: ## Check formatting of project Go files
	@test -z "$$(gofmt -l $(GOFILES))"

test-example: ## Verify parser extraction of the inline example
	go test ./internal/parser -run '^TestParseInlineExample$$' -v -count=1

test-unit: ## Run fast tests without integrations
	go test -short ./...

test-integration: ## Run generator, runner, and CLI integration tests
	go test ./... -run '^TestIntegration' -count=1

test: ## Run the full test suite
	go test ./...

test-race: ## Run the full test suite with the race detector
	go test -race ./...

vet: ## Run go vet
	go vet ./...

build: ## Build bin/goit without creating a root-level artifact
	@mkdir -p bin
	go build -o bin/goit ./cmd/goit

run: ## Run goit on FILE (default: examples/sum/main.go)
	go run ./cmd/goit -- "$(FILE)"

generate-only: build ## Create and keep FILE's _test.go without running tests
	./bin/goit --generate-only -- "$(FILE)"

test-inline: build ## Run the sum example's inline tests
	./bin/goit examples/sum/main.go

check: fmt-check test-example test-unit test-integration test-race vet build ## Run all validation checks
