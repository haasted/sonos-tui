.PHONY: build vet test test-verbose test-race run tidy

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

run:
	go run ./cmd/sonos

tidy:
	go mod tidy
