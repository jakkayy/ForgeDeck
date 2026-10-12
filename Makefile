# Local settings (see .env.example); optional.
-include .env
export

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO_BUILD = CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: fmt lint test generate web build build-api build-agent migrate-up migrate-down migrate-status

fmt:
	golangci-lint fmt

lint:
	golangci-lint run ./...

test:
	go test -race ./...

generate:
	sqlc generate

# web builds the dashboard and stages it for embedding into the API binary.
web:
	cd apps/web && bun install --frozen-lockfile && bun run build
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
	cp -R apps/web/dist/. internal/webui/dist/

build: web build-api build-agent

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
