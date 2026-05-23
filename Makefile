.PHONY: build test lint clean release-dry run

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/eslusarenko/port-server/internal/version.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/port-server ./cmd/port-server

test:
	go test -race ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/ dist/

release-dry:
	goreleaser release --snapshot --clean

run:
	go run ./cmd/port-server
