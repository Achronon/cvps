.PHONY: build test lint clean install

VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X github.com/achronon/cvps/internal/version.Version=$(VERSION) \
           -X github.com/achronon/cvps/internal/version.Commit=$(COMMIT)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/cvps ./cmd/cvps

test:
	go test -v ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/

install: build
	cp bin/cvps /usr/local/bin/
