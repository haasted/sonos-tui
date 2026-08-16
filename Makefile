.PHONY: build test test-verbose run tidy

build:
	go build ./...

test:
	go test ./...

test-verbose:
	go test ./... -v

run:
	go run ./cmd/sonos

tidy:
	go mod tidy
