.PHONY: build fmt fmt-check vet tidy-check test test-verbose test-race test-live run run-logs run-nocache db queue-events notify-capture discover-probe music-services sonos-web uitest worktrees tidy adr-html help

.DEFAULT_GOAL := help

# Every Go module in the repository. The experiments are a module of their own,
# so ./... from the root does not reach them and each target must say so.
# CI passes MODULES=. to check the program alone.
MODULES ?= . ./experiments

build: ## Build the binary
	CGO_ENABLED=0 go build ./cmd/sonos

fmt: ## Format all Go files in place (gofmt -w), listing any it touched
	@for m in $(MODULES); do (cd $$m && gofmt -l -w $$(go list -f '{{.Dir}}' ./... | sort -u)) || exit 1; done

fmt-check: ## List Go files gofmt would change, and fail if there are any
	@for m in $(MODULES); do (cd $$m && out=$$(gofmt -l $$(go list -f '{{.Dir}}' ./... | sort -u)) && \
	  if [ -n "$$out" ]; then echo "$$out"; exit 1; fi) || exit 1; done

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

notify-capture: ## Record the raw NOTIFY bodies of one event service
	cd experiments && go run ./notifycapture $(NOTIFY_ARGS)

discover-probe: ## Look for speakers for 10s by SSDP and mDNS on every interface, logging all
	cd experiments && go run ./discoverprobe $(PROBE_ARGS)

music-services: ## List the music services one speaker offers (MUSIC_ARGS=-ip=<address>)
	cd experiments && go run ./musicservices $(MUSIC_ARGS)

sonos-web: ## Sign in to the Sonos web app, asking for the account (WEB_ARGS=-fresh signs in again)
	cd experiments && go run ./sonosweb $(WEB_ARGS)

run-logs: ## Run the CLI and print the logs once it exits
	go run ./cmd/sonos -logs

run-nocache: ## Delete speaker cache prior to starting
	rm -f ~/Library/Application\ Support/sonos-sandbox/sonos.db*
	go run ./cmd/sonos

db: ## Open the database in lazysql, with the connection in .lazysql.toml
	@command -v lazysql >/dev/null || { echo "lazysql is not installed (brew install lazysql)"; exit 1; }
	lazysql -config .lazysql.toml

tidy-check: ## Fail if go.mod/go.sum are not tidy, showing what tidy would change
	@for m in $(MODULES); do (cd $$m && go mod tidy -diff) || exit 1; done

tidy: ## Tidy go.mod/go.sum
	@for m in $(MODULES); do (cd $$m && go mod tidy) || exit 1; done

worktrees: ## Report the state of every worktree. Removes nothing.
	./scripts/worktree_report.sh

adr-html: ## Render ADRs/*.md to standalone HTML pages with diagrams in ADRs/html/.
	./scripts/render_adr_html.sh

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'
