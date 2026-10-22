# Local settings (see .env.example); optional.
-include .env
export

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO_BUILD = CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: fmt lint test generate web build build-api build-agent build-runner migrate-up migrate-down migrate-status \
	db-up db-down dev-api dev-web create-user create-runner run-runner

fmt:
	golangci-lint fmt

lint:
	golangci-lint run ./...

test:
	go test -race ./...

generate:
	sqlc generate

web:
	cd apps/web && bun install --frozen-lockfile && bun run build
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
	cp -R apps/web/dist/. internal/webui/dist/

build: web build-api build-agent build-runner

build-api:
	$(GO_BUILD) -o bin/forgedeck-api ./cmd/api

build-agent:
	$(GO_BUILD) -o bin/forgedeck-agent ./cmd/agent

build-runner:
	$(GO_BUILD) -o bin/forgedeck-runner ./cmd/runner

migrate-up:
	go run ./cmd/api migrate up

migrate-down:
	go run ./cmd/api migrate down

migrate-status:
	go run ./cmd/api migrate status

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

dev-api:
	go run ./cmd/api serve

dev-web:
	cd apps/web && bun install --frozen-lockfile && bun run dev

create-user:
	@test -n "$(EMAIL)" || { echo "usage: make create-user EMAIL=you@example.com"; exit 1; }
	go run ./cmd/api create-user --email "$(EMAIL)"

# create-runner NAME=build-01 prints the Runner credential once.
create-runner:
	@test -n "$(NAME)" || { echo "usage: make create-runner NAME=build-01"; exit 1; }
	go run ./cmd/api create-runner --name "$(NAME)"

# run-runner URL=http://localhost:8080 (credential in FORGEDECK_RUNNER_CREDENTIAL); uses your own Docker daemon
run-runner: build-runner
	bin/forgedeck-runner run --url "$(or $(URL),http://localhost:8080)" --allow-insecure --work-dir "$(or $(WORK_DIR),/tmp/forgedeck-runner)"
