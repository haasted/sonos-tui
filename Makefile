.PHONY: build fmt vet test test-verbose test-race test-live run run-logs run-capture run-nocache queue-events uitest worktrees tidy adr-html help

.DEFAULT_GOAL := help

# Every Go module in the repository. The experiments are a module of their own,
# so ./... from the root does not reach them and each target must say so.
MODULES := . ./experiments

build: ## Build the binary
	CGO_ENABLED=0 go build ./cmd/sonos

fmt: ## Format all Go files in place (gofmt -w), listing any it touched
	@for m in $(MODULES); do (cd $$m && gofmt -l -w $$(go list -f '{{.Dir}}' ./... | sort -u)) || exit 1; done

vet: ## Run go vet
	@for m in $(MODULES); do (cd $$m && go vet ./...) || exit 1; done

test: ## Run tests (never cached)
	@for m in $(MODULES); do (cd $$m && go test -count=1 ./...) || exit 1; done

test-verbose: ## Run tests with output enabled
	@for m in $(MODULES); do (cd $$m && go test ./... -v) || exit 1; done

test-race: ## Run tests with the race detector
	@for m in $(MODULES); do (cd $$m && go test ./... -race) || exit 1; done

test-live: ## Run tests requiring a real network with real Sonos speakers
	go test ./internal/sonos/... -live -v
	cd experiments && go test ./ssdp/... -live -v
	cd experiments && go test ./mdns/... -live -v

run: ## Run the CLI
	go run ./cmd/sonos

# make uitest <name> reads the name as a second goal. Turn any word after
# uitest into the demo to run, and into a target that does nothing, so make does
# not then look for a rule for it. Guarded on uitest being the FIRST goal, so a
# mistyped target anywhere else still fails as it should.
ifeq (uitest,$(firstword $(MAKECMDGOALS)))
UITEST_LAYOUT := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
$(eval $(UITEST_LAYOUT):;@:)
endif

uitest: ## Show a UI demo: make uitest <name>. Without a name, the demos are listed.
	@cd experiments && if [ -z "$(UITEST_LAYOUT)" ]; then \
	  go run ./uitest 2>&1 | grep -v '^exit status'; \
	else \
	  go run ./uitest $(UITEST_LAYOUT); \
	fi

queue-events: ## Watch ContentDirectory events while a queue is edited
	cd experiments && go run ./queueevents

run-logs: ## Run the CLI and print the logs once it exits
	go run ./cmd/sonos -logs

run-capture: ## Capture speaker data while running
	go run ./cmd/sonos -capture=./captured-data

run-nocache: ## Delete speaker cache prior to starting
	rm -f ~/Library/Caches/sonos-sandbox/speakers.json
	go run ./cmd/sonos

tidy: ## Tidy go.mod/go.sum
	@for m in $(MODULES); do (cd $$m && go mod tidy) || exit 1; done

worktrees: ## Report the state of every worktree. Removes nothing.
	./scripts/worktree_report.sh

adr-html: ## Render ADRs/*.md to standalone HTML pages with diagrams in ADRs/html/.
	./scripts/render_adr_html.sh

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'
