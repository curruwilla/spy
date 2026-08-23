BINARY := spy
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build run test lint fmt cover install clean help

## build: compile the binary into bin/
build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/spy

## run: build and start the monitor
run: build
	./bin/$(BINARY)

## test: run the test suite with the race detector
test:
	go test -race ./...

## cover: report test coverage per package
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

## lint: vet and, when installed, golangci-lint
lint:
	go vet ./...
	@command -v golangci-lint >/dev/null && golangci-lint run || echo "golangci-lint unavailable, skipped"

## fmt: format the source
fmt:
	gofmt -s -w .

## install: install spy into GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/spy

## clean: remove build artifacts
clean:
	rm -rf bin coverage.out

## help: list the targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
