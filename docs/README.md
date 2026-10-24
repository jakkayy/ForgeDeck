# ForgeDeck

> Self-hosted DevOps Control Plane for Docker-based applications

ForgeDeck is a self-hosted DevOps platform for managing applications, Docker servers, Git repositories, CI/CD pipelines and deployments from one dashboard.

The goal is to make deploying Docker-based applications easy, without having to SSH into a server every time.

> Deploy your Docker applications without managing the infrastructure manually.

This file is the overview. The details and the contracts used in the implementation are in the separate documents below; if they disagree, the separate documents win.

---

## Documents

| File | Contents |
|---|---|
| [01 — Product Spec](01-product-spec.md) | Problem, users, MVP scope, non-goals |
| [02 — Architecture](02-architecture.md) | Components, trust boundaries, job lifecycle |
| [03 — Database](03-database.md) | Schema, constraints, queue transaction |
| [04 — API Spec](04-api-spec.md) | REST API, SSE, Agent endpoints |
| [05 — Agent Protocol](05-agent-protocol.md) | Enrollment, heartbeat, snapshot, long-poll, job payloads |
| [06 — CI/CD Pipeline](06-ci-cd-pipeline.md) | Webhook → build → push → deploy → health check |
| [07 — Security](07-security.md) | Threat model and production checklist |
| [08 — Frontend UX](08-frontend-ux.md) | Navigation, main pages, design system, performance |
| [09 — Development Guide](09-development-guide.md) | Local setup, testing, PR checklist |
| [10 — Roadmap](10-roadmap.md) | Phases and acceptance criteria |
| [11 — Implementation Plan](11-implementation-plan.md) | Sub-items and commits of each phase |
| [12 — Measurements](12-measurements.md) | Latency and resource measurements |
| [13 — Git Providers](13-git-providers.md) | Setting up the GitHub App and GitLab |
| [14 — Runner](14-runner.md) | Runner protocol, build steps and isolation |
| [15 — Deployments](15-deployments.md) | Environment, deployment, release, rollback, service configuration |
| [16 — Backup and Restore](16-backup-restore.md) | Backing up and restoring the database, the keys to store separately, how to test it |
| [17 — Upgrade Guide](17-upgrade-guide.md) | Upgrading the Control Plane, Agent and Runner, rolling back, changes to follow |
| [AGENTS.md](../AGENTS.md) | Rules for AI coding agents |

---

## What ForgeDeck is and is not

**Is** a lightweight self-hosted control plane that keeps this whole flow in one place:

```text
Code → Build → Test → Image → Registry → Deploy → Health Check → Release
```

**Is not** a Kubernetes platform, Git hosting, a cloud provider or a general-purpose CI engine.

The advantage comes not from the number of features but from:

```text
Simplicity + Speed + Self-hosted + Docker-native + Developer UX
```

---

## Principles

1. **Simple** — users can deploy without understanding all the infrastructure
2. **Lightweight** — the MVP uses no Kubernetes, Kafka, Redis, Temporal, Elasticsearch, Prometheus, Grafana
3. **Fast** — server-side pagination, SSE only on the pages that need it, no system-wide polling
4. **Secure by default** — Docker Engine is never exposed as a public API; the Agent is always the intermediary
5. **Developer friendly** — every page answers "what state is the system in right now?" at a glance

---

## Architecture

```text
              ┌────────────────────┐
              │      Browser       │
              │ React SPA + HeroUI │
              └─────────┬──────────┘
                        │
                        ▼
              ┌────────────────────┐        GitHub / GitLab
              │  Go Control Plane  │◄────── webhooks
              │ (modular monolith) │
              └──┬──────────────┬──┘
                 │              │
                 ▼              │ jobs (pull)
          ┌────────────┐        │
          │ PostgreSQL │   ┌────┴─────────────┐
          │ data+queue │   │                  │
          └────────────┘   ▼                  ▼
                       ┌────────┐       ┌──────────┐
                       │ Runner │       │  Agent   │  outbound HTTPS
                       └───┬────┘       └────┬─────┘  (long-poll)
                           │ push            │ pull
                           ▼                 ▼
                       ┌──────────┐   ┌───────────────┐
                       │ Registry │──►│ Docker Engine │
                       └──────────┘   │  App A, B, C  │
                                      └───────────────┘
```

| Component | Role | Details |
|---|---|---|
| Control Plane | Auth, projects, servers, jobs, deployments, secrets, audit | [02](02-architecture.md) |
| PostgreSQL | Source of truth and job queue (`SKIP LOCKED`) | [03](03-database.md) |
| Runner | Checkout → test → build → push, separated from production | [06](06-ci-cd-pipeline.md) |
| Agent | Controls Docker on a server, sends heartbeats and container snapshots | [05](05-agent-protocol.md) |
| Web UI | Vite + React SPA, HeroUI, TanStack Query — built as static files and embedded in the Go binary | [08](08-frontend-ux.md) |

Key points:

- **A server belongs to a project** — every permission check goes through `servers.project_id`
- **The Agent uses a pull model** — a server needs no inbound port and can sit behind NAT
- **The Runner is separate from production** — source code is untrusted input, and the Docker socket must never be mounted
- **Deploy by image digest** — a rollback reuses the previous digest and does not rebuild
- **The Control Plane is one binary** — the web UI is embedded in the Go binary, so production needs no JavaScript runtime (Bun is used only at build time)

---

## Golden Path

With the defaults, a user must be able to deploy without configuring much:

```text
Login → Create Project → Connect GitHub → Select Repository
  → Create Application → Add Server (install Agent)
  → Push Code → Watch Build → Watch Deployment → Healthy
```

---

## MVP Definition of Done

The MVP is done when a user can do all of these:

1. Login
2. Create a project
3. Connect GitHub/GitLab
4. Select a repository
5. Create an application
6. Add a Docker server
7. Install Agent
8. The Agent comes online
9. Push Code
10. The webhook reaches ForgeDeck
11. A Runner takes the job
12. Checkout Code
13. Run Test
14. Build Docker Image
15. Push Registry
16. The Agent takes the deployment
17. Pull Image
18. Start Container
19. Health Check
20. The deployment shows as Healthy
21. View logs
22. View the release history
23. Rollback Release

The first milestone is **"Manage a remote Docker server from the ForgeDeck Dashboard"**. See each phase in [10 — Roadmap](10-roadmap.md)

---

## Resource goals

ForgeDeck (Control Plane + PostgreSQL + Runner) should run on a machine with **2 CPU / 4 GB RAM** for development and a small production workload

The applications' own workload depends on the servers that run an Agent.

---

## Repository layout

A monorepo:

```text
forgedeck/
├── apps/web/          Vite + React dashboard (embedded in cmd/api)
├── cmd/
│   ├── api/           Control Plane
│   ├── runner/        CI Runner
│   └── agent/         Docker Agent
├── internal/          Go modules (auth, projects, servers, agents, jobs, ...)
├── migrations/        SQL migrations
├── packages/api-client/
├── deploy/            Compose and Runner setup
└── docs/
```

Details and development commands are in [09 — Development Guide](09-development-guide.md)

---

## Philosophy

> Don't build infrastructure for the sake of infrastructure.

- If PostgreSQL does the job, do not add Redis
- If Docker Compose does the job, do not add Kubernetes
- If an Agent over HTTPS does the job, do not add a message broker
- If SSE does the job, do not add a complex streaming system

> Simple until complexity is actually needed.

The goal is a system that is **as easy as possible, as light as possible, and actually deploys**.
