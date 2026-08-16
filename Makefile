.PHONY: build vet test test-verbose test-race test-live run tidy

build:
	CGO_ENABLED=0 go build ./...

vet:
	go vet ./...

test:
	go test ./...

test-verbose:
	go test ./... -v

test-race:
	go test ./... -race

test-live:
	go test ./internal/sonos/... -live -v

run:
	go run ./cmd/sonos

tidy:
	go mod tidy
