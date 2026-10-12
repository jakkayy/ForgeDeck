# 02 — Architecture

## 1. Architecture goals

- Keep the system small and understandable.
- Separate orchestration from code execution.
- Do not require inbound access to production servers.
- Continue running deployed applications if the Control Plane is temporarily unavailable.
- Allow each customer to run an independent ForgeDeck installation.

## 2. Components

### Web UI

Next.js App Router, TypeScript, Tailwind CSS, HeroUI, and TanStack Query. It presents projects, applications, servers, jobs, releases, logs, and settings.

### Control Plane API

A Go modular monolith exposing REST APIs under `/api/v1`. It owns authentication, authorization, metadata, Git integration, webhook processing, scheduling, deployment orchestration, secrets, and audit records.

### PostgreSQL

The source of truth for application metadata, jobs, leases, releases, users, and audit events. Use transactions and constraints to protect state transitions. PostgreSQL is the initial job queue; a separate broker is not required for MVP.

### Runner

A separate Go process or service that executes build/test jobs in an isolated workspace. It checks out a fixed commit, runs configured steps, builds an image, and pushes it to a registry. It must not control the production Docker Engine.

### Agent

A small Go process installed on each target server. It initiates outbound HTTPS connections to the Control Plane, reports health and container snapshots, claims jobs targeted at its server through a long-poll endpoint, operates the local Docker Engine, and sends job events/results. Each server and its Agent belong to one project.

### Container registry

Stores built images. Initial integrations may include GHCR and GitLab Container Registry. Deployments should record image digests.

## 3. Logical diagram

```text
Browser
  │ HTTPS
  ▼
Next.js Web UI ───────► Go Control Plane ───────► PostgreSQL
                              │
                              ├── GitHub/GitLab webhooks
                              │
                              ├── schedules build jobs
                              ▼
                            Runner ─────────────► Registry
                                                   │
                                                   ▼
Production Agent ◄──── outbound HTTPS ───── Control Plane
  │
  ▼
Local Docker Engine / Docker Compose
  │
  └── Application containers
```

## 4. Trust boundaries

1. **Browser → Control Plane:** untrusted client; authenticate and authorize every operation.
2. **Git provider → webhook endpoint:** verify signatures/secrets and deduplicate deliveries.
3. **Repository → Runner:** treat source code, dependencies, and build scripts as untrusted.
4. **Control Plane → Agent:** authenticate Agent identity and validate every job payload.
5. **Agent → Docker Engine:** high privilege on the target host; restrict the Agent's supported operations and protect its credentials.
6. **Control Plane → registry/provider:** use least-privilege credentials and avoid logging them.

## 5. Control Plane design

Use a modular monolith with internal packages and clear interfaces. Suggested modules:

- `auth`
- `projects`
- `applications`
- `gitprovider`
- `webhooks`
- `jobs`
- `pipelines`
- `deployments`
- `servers`
- `agents`
- `docker`
- `secrets`
- `audit`

Keep business rules out of HTTP handlers. Handlers validate/translate HTTP requests; domain/service code enforces rules; repositories handle persistence.

## 6. Job lifecycle

A job is stored before it is dispatched. A worker claims it transactionally, receives a lease, reports progress, and completes it. If a worker disappears and the lease expires, the scheduler can make the job eligible for retry according to job type and retry policy.

Delivery is at-least-once in practical failure scenarios. Operations must therefore be idempotent or detect already-completed work. Never assume exactly-once execution.

## 7. Availability behavior

- Running application containers should not depend on continuous Control Plane availability.
- The Agent should reconnect after network interruption.
- A lost Runner/Agent must not leave a job permanently running; leases and recovery handle this.
- Database backups and restore procedures are required before production use.
- Control Plane upgrades must not silently discard in-progress job state.

## 8. Deployment model

Each installation runs its own Control Plane and database. A basic installation may use Docker Compose for `web`, `api`, `postgres`, and optionally `runner`. Production servers run the Agent separately.

## 9. Technology decisions

| Concern | Initial choice | Rationale |
|---|---|---|
| Backend | Go | Simple deployment, concurrency, good Docker ecosystem |
| HTTP router | Chi | Small, idiomatic routing layer |
| Database access | pgx + sqlc | Explicit SQL and typed generated access |
| Database | PostgreSQL | Metadata, transactions, and initial queue |
| Frontend | Next.js + TypeScript | Structured dashboard and mature ecosystem |
| UI | HeroUI + Tailwind | Consistent components and fast UI iteration |
| Live logs | SSE | One-way server-to-browser stream |
| Runtime | Docker Engine / Compose | Matches MVP scope |
| Build | BuildKit/Buildx in isolation | Efficient image builds without sharing production Docker access |

## 10. Open architecture decisions

- Exact process topology for Runner: same host as Control Plane or separate host.
- Whether log chunks live in PostgreSQL initially or move to object storage later.
- Supported Compose deployment/update strategy for services with persistent volumes.
- How image registry credentials are delivered to Runner and Agent.
