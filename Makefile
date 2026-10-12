# Local settings (see .env.example); optional.
-include .env
export

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO_BUILD = CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: fmt lint test generate build build-api build-agent migrate-up migrate-down migrate-status

fmt:
	golangci-lint fmt

lint:
	golangci-lint run ./...

test:
	go test -race ./...

generate:
	sqlc generate

build: build-api build-agent

build-api:
	$(GO_BUILD) -o bin/forgedeck-api ./cmd/api

build-agent:
	$(GO_BUILD) -o bin/forgedeck-agent ./cmd/agent

migrate-up:
	go run ./cmd/api migrate up

migrate-down:
	go run ./cmd/api migrate down

migrate-status:
	go run ./cmd/api migrate status
