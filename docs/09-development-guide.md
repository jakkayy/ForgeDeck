# 09 — Development Guide

## 1. Status

This is a planned development setup. Exact commands depend on the repository's package manifests, Go module, and Compose configuration once those files exist.

## 2. Prerequisites

Recommended local tools:
- Git
- Go toolchain (version from `go.mod`)
- golangci-lint v2.5 (`make lint`, `make fmt`)
- sqlc v1.30 (`make generate`): `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0`
- Bun (build-time only for the Web UI; not needed to run ForgeDeck)
- Docker Engine
- Docker Compose plugin
- PostgreSQL (or the project Compose service)
- A GitHub/GitLab test repository
- A test container registry

Record exact supported versions in the repository once the first working setup is committed.

## 2a. Quick start with make

```sh
cp .env.example .env            # set the database password and FORGEDECK_SECRET_KEYS
make db-up                      # PostgreSQL
make dev-api                    # Control Plane on :8080 (migrates on start)
make create-user EMAIL=you@example.com
make dev-web                    # dashboard with hot reload, proxying to :8080
```

For a Runner: `make create-runner NAME=build-01`, then
`FORGEDECK_RUNNER_CREDENTIAL=fdr_… make run-runner` (optional `URL=` and `WORK_DIR=`).
`make build` produces the single-binary build in `bin/`; `make db-down` stops PostgreSQL.

## 2b. End-to-end test

`internal/e2e` runs the product's golden path with real components: a webhook push, the Runner (git over HTTP, BuildKit build, tests, push to a throwaway registry), an Agent managing the local Docker through Compose, an automatic deployment with an HTTP health check, a second version, a version that never becomes healthy (the previous one must keep serving and no release may be recorded), and a rollback. It needs Docker, the Compose plugin, git and git-http-backend, so it runs only with `FORGEDECK_TEST_DOCKER=1` (about 80 seconds):

```sh
FORGEDECK_TEST_DOCKER=1 go test ./internal/e2e/
```

## 3. Suggested repository layout

```text
forgedeck/
├── apps/
│   └── web/
├── cmd/
│   ├── api/
│   ├── runner/
│   └── agent/
├── internal/
│   ├── auth/
│   ├── projects/
│   ├── applications/
│   ├── environments/
│   ├── gitprovider/
│   ├── webhooks/
│   ├── jobs/
│   ├── pipelines/
│   ├── deployments/
│   ├── servers/
│   ├── agents/
│   ├── docker/
│   ├── secrets/
│   └── audit/
├── migrations/
├── packages/
│   └── api-client/
├── deploy/
│   ├── compose/
│   └── runner/
├── docs/
├── README.md
├── AGENTS.md
├── docker-compose.yml
└── Makefile
```

Adjust this layout if implementation reveals a simpler boundary. Do not create empty abstractions without a use case.

## 4. Local development principles

- Start with PostgreSQL and the API.
- Add the Web UI after the first API contract is testable.
- During development, run the Vite dev server and proxy `/api` to the Go API so the browser still sees a single origin. Production builds embed `apps/web/dist` into the API binary.
- Develop Agent operations against a disposable test Docker host first.
- Use a separate test Runner environment; never test untrusted build code on a production machine.
- Keep local secrets in ignored environment files or a secret manager.
- Provide an `.env.example` containing names and safe placeholders only.

## 5. Database workflow

- Store SQL migrations in `migrations/`.
- Apply migrations through a repeatable command or migration tool.
- Generate `sqlc` code from committed SQL queries.
- Test migrations from an empty database and, where relevant, from the prior released schema.
- Never edit a production database manually as the normal schema-change workflow.

## 6. Suggested Makefile commands

Names can be adjusted as implementation lands:

```make
.PHONY: test lint fmt generate migrate-up web-dev

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

web-dev:
	cd apps/web && bun run dev
```

Add lint, SQL generation, migrations, and build commands when their tools are selected. Do not include commands that do not actually work.

## 7. Testing layers

### Unit tests
- Job state transitions.
- Authorization policies.
- Webhook validation.
- Retry and lease calculations.
- Deployment state transitions.
- Secret redaction.

### Integration tests
- PostgreSQL transactions and concurrent job claims.
- Agent authentication and revocation.
- Docker operations against a disposable daemon.
- SSE reconnect and event ordering.

### End-to-end tests
1. Create project.
2. Connect test repository.
3. Register a test server.
4. Trigger a build.
5. Verify image pushed.
6. Deploy to test environment.
7. Verify health check.
8. View logs and release.
9. Roll back to a previous digest.

## 8. Development security

- Do not commit real `.env` files, tokens, private keys, or exported workflows containing credentials.
- Use test provider apps and test registry credentials.
- Keep production and test servers separate.
- Never use a public, unauthenticated Docker TCP endpoint.
- Review logs before sharing them.

## 9. Pull request checklist

- [ ] Scope matches a roadmap milestone.
- [ ] Tests/checks run and results are reported.
- [ ] API/database docs updated if contracts changed.
- [ ] Authorization and error paths tested.
- [ ] No credentials in source, logs, or fixtures.
- [ ] Resource/time limits considered for background work.
- [ ] UI loading, empty, and failure states included where applicable.
- [ ] Migrations are safe and reversible where feasible.

## 10. First local milestone

Build a minimal vertical slice:
- PostgreSQL starts.
- Go API health endpoint works.
- A server/Agent can authenticate in a test setup.
- Agent reports heartbeat.
- UI shows the server's online/offline status.

Only after that is reliable should container actions and CI/CD be added.
