# ForgeDeck

**English** · [ไทย](README.th.md)

> Self-hosted DevOps control plane for Docker: Code → Build → Test → Image → Registry → Deploy

ForgeDeck manages applications, Docker servers, Git repositories and CI/CD from one dashboard, so you do not have to SSH into a server for every deployment. It is a single Go binary with the UI embedded, and uses PostgreSQL as both the database and the job queue. The overview and the reasoning behind the design are in [docs/README.md](docs/README.md).

## Status

> **Early software.** The installers and the Agent/Runner systemd units have been exercised in containers and on one home server, not yet by an end-to-end test on a real systemd host in CI. Do not rely on it for anything you cannot afford to lose. Security reports: [SECURITY.md](SECURITY.md).

| Phase | Scope | Status |
|---|---|---|
| 0–1 | Project skeleton, auth, projects, servers + Agent, container management | Done |
| 2 | Git integration (GitHub/GitLab), webhooks, job queue | Done |
| 3 | Runner separated from production: checkout → build → test → push | Done |
| 4 | Environments, deployment through the Agent + Compose, health check, releases, rollback | Done |
| 5 | Secrets, audit log, RBAC, rate limits, retention, backup, production Compose, Agent installer, upgrade guide | Done |
| 6 | Overview, status pages, pagination/filters, virtualized logs, accessibility, benchmarks | Done |
| 7 | Ease of use: installation administrator, Runners from the dashboard, one-command Agent and Runner install (more is planned) | In progress |

Details per item: [docs/11-implementation-plan.md](docs/11-implementation-plan.md).

## Components

| Component | Role | Binary |
|---|---|---|
| Control Plane | API, web UI, job queue, auth, secrets | `forgedeck-api` |
| Agent | Runs on a Docker server and controls its containers (connects *out* to the Control Plane) | `forgedeck-agent` |
| Runner | Runs on a separate build host: checkout → build → test → push | `forgedeck-runner` |
| PostgreSQL | Data and job queue | via Docker Compose |

## Install (production)

Use [deploy/compose](deploy/compose/README.md): the Control Plane (the image `ghcr.io/jakkayy/forgedeck-api` of a release), PostgreSQL and Caddy (automatic TLS) in one Compose project. Only the proxy publishes ports. **Keep `FORGEDECK_SECRET_KEYS` away from that host**: it is not in backups, and without it stored Git tokens and secrets cannot be read ([docs/16-backup-restore.md](docs/16-backup-restore.md)).

The first visit shows **Create the first account**; that account owns the installation and the page closes for good once an account exists. If the host is reachable from the internet before you register, set `FORGEDECK_DISABLE_WEB_SETUP=true` and create the account with `forgedeck-api create-user` instead.

### Add a server (Agent)

In the dashboard open **Servers → Add server**. It shows a one-line command that downloads the Agent from the release, verifies its checksum and enrolls it. Run it on a Linux host with systemd, Docker and the Compose plugin. Details and the privileges involved: [deploy/agent](deploy/agent/README.md).

### Add a build host (Runner)

An administrator opens **Runners → Add Runner**, which shows one command to run on a dedicated build host (Debian or Ubuntu with Docker's apt repository). It installs the Runner, gives it its own rootless Docker daemon and starts it. The Runner executes untrusted repository code, so keep it away from production: [deploy/runner](deploy/runner/README.md), [docs/14-runner.md](docs/14-runner.md).

## First build and deploy

1. Create a project, open its **Git** tab and connect GitHub or GitLab (a GitLab token needs the `api` scope; see [docs/13-git-providers.md](docs/13-git-providers.md)).
2. On **Registries** add a registry (GitHub, GitLab or Docker Hub presets are offered, or any registry such as `localhost:5000`).
3. On **Applications** create an application: repository, branch, registry and image name. The repository needs a `Dockerfile`.
4. Open the application and press **Build** (or push to the branch). A build produces an image reference such as `host/image@sha256:…`.
5. Press **New environment** (server, container port, host port, optional health check), then **Deploy**. A deployment is recorded as a release only when the health check passes; if the new version is not healthy the previous one is restored, and **Roll back** returns to an older release. Data in volumes and databases is not rolled back ([docs/15-deployments.md](docs/15-deployments.md)).

## Development

Requirements: Go (version in `go.mod`), Docker Engine with the Compose plugin, git and [Bun](https://bun.sh) (UI build only). Optional: `golangci-lint` v2.5 (`make lint`), `sqlc` v1.30 (`make generate`).

```sh
cp .env.example .env     # set POSTGRES_PASSWORD, FORGEDECK_DATABASE_URL, FORGEDECK_SECRET_KEYS
make db-up               # PostgreSQL
make dev-api             # Control Plane on :8080 (applies migrations)
make dev-web             # UI on http://localhost:5173 (hot reload)
```

Create the encryption key with `openssl rand -base64 32` and set `FORGEDECK_SECRET_KEYS=v1:<value>`. `make build` builds the web UI, API, Agent and Runner into `bin/`.

To try a Runner locally (it uses your own Docker daemon, so for experiments only):

```sh
make create-runner NAME=build-01          # prints the credential (fdr_...) once
FORGEDECK_RUNNER_CREDENTIAL=fdr_... make run-runner URL=http://localhost:8080
```

| Command | What it does |
|---|---|
| `make test` | Runs all tests (set `FORGEDECK_TEST_DATABASE_URL`; add `FORGEDECK_TEST_DOCKER=1` for the Docker-based tests) |
| `make lint` / `make fmt` | Checks / formats the Go code |
| `make generate` | Generates the sqlc code from `internal/store/queries` |
| `make migrate-status` | Shows the migration status |
| `make db-down` | Stops PostgreSQL |

## Important settings

All are listed in [.env.example](.env.example).

| Variable | Meaning |
|---|---|
| `FORGEDECK_DATABASE_URL` | PostgreSQL connection string |
| `FORGEDECK_SECRET_KEYS` | Secret-encryption keys as `id:base64,id:base64` (the first encrypts new data) |
| `FORGEDECK_PUBLIC_URL` | The URL GitHub/GitLab call back for webhooks |
| `FORGEDECK_ALLOW_PRIVATE_GIT_HOSTS` | Allow Git servers on internal networks (off by default, to prevent SSRF) |
| `FORGEDECK_DISABLE_WEB_SETUP` | Disable the first-account web page |

## Documentation

| File | Contents |
|---|---|
| [01 Product Spec](docs/01-product-spec.md) · [02 Architecture](docs/02-architecture.md) | Scope and architecture |
| [03 Database](docs/03-database.md) · [04 API](docs/04-api-spec.md) | Schema and REST API |
| [05 Agent Protocol](docs/05-agent-protocol.md) · [14 Runner](docs/14-runner.md) | Agent and Runner protocols |
| [06 CI/CD](docs/06-ci-cd-pipeline.md) · [07 Security](docs/07-security.md) | Pipeline and threat model |
| [08 Frontend](docs/08-frontend-ux.md) · [09 Development](docs/09-development-guide.md) | UI and development guide |
| [10 Roadmap](docs/10-roadmap.md) · [11 Plan](docs/11-implementation-plan.md) | Plans |
| [12 Measurements](docs/12-measurements.md) · [13 Git Providers](docs/13-git-providers.md) | Measurements and provider setup |
| [15 Deployments](docs/15-deployments.md) | Environments, deployments, releases, rollback |
| [16 Backup and Restore](docs/16-backup-restore.md) | Backing up and restoring; `FORGEDECK_SECRET_KEYS` is kept separately |
| [17 Upgrade Guide](docs/17-upgrade-guide.md) | Upgrading, rolling back, changes to follow |
| [AGENTS.md](AGENTS.md) | Rules for AI coding agents |
