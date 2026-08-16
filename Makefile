.PHONY: build test run tidy

build:
	go build ./...

test:
	go test ./... -v

run:
	go run ./cmd/sonos

tidy:
	go mod tidy
