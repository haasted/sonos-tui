.PHONY: build vet test test-verbose test-race test-live run tidy adr-html help

.DEFAULT_GOAL := help

build: ## Build the binary
	CGO_ENABLED=0 go build ./...

vet: ## Run go vet
	go vet ./...

test: ## Run tests
	go test ./...

test-verbose: ## Run tests with output enabled
	go test ./... -v

test-race: ## Run tests with the race detector
	go test ./... -race

test-live: ## Run tests requiring a real network with real Sonos speakers
	go test ./internal/sonos/... -live -v

run: ## Run the CLI
	go run ./cmd/sonos

tidy: ## Tidy go.mod/go.sum
	go mod tidy

adr-html: ## Render ADRs/*.md to standalone HTML pages with diagrams in ADRs/html/.
	./scripts/render_adr_html.sh

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'
