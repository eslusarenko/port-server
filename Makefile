.PHONY: build test lint clean release-dry run

build:
	go build -o bin/port-server ./cmd/port-server

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
