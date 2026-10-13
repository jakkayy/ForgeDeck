# 15 — Deployments and Releases

How an image built by the Runner ([14](14-runner.md)) reaches a server and becomes a recorded, roll-back-able release. This document is the contract for Phase 4; it resolves the open decisions in [06 §13](06-ci-cd-pipeline.md) and [03 §7](03-database.md).

## 1. Model

```text
application ──< environment ──< deployment ──(healthy)──> release
                    │
                    └── one server, one service
```

| Term | Meaning |
|---|---|
| **Environment** | A named target of an application (`production`, `staging`) bound to **one server** in the same project and described by one service configuration (§ [2](#2-service-configuration-4-2)). Name is unique per application. |
| **Deployment** | One attempt to put one image digest into one environment. It is tied to a `deploy_image` job and ends `succeeded` or `failed`. Every attempt is kept, including failures. |
| **Release** | A deployment that passed its health check, recorded immutably with commit, image digest, configuration snapshot, and time. Failed deployments are never releases. |
| **Current release** | The environment's most recent release. It is what is running (or should be). |

Rules:

- A deployment always names an immutable **digest** (`registry/repo@sha256:…`), taken from a succeeded build's result. A mutable tag is never deployed.
- A deployment is created from either a **succeeded build job** or an **earlier release** (rollback). Rollback is a normal deployment whose source is the old release; it produces a new release on success. History is append-only; nothing is rewritten.
- Deployments to one environment run in creation order. They are `deploy_image` jobs targeted at the environment's server, and the Agent runs its jobs one at a time, so there is no separate lock. A newer deployment queued behind a running one waits.
- A release stores a **snapshot** of the configuration it ran with, so rolling back restores the old configuration together with the old image, not just the image.
- Release records are kept as long as the environment exists. Deleting an environment deletes its history; there is no pruning in the MVP.

### Triggers

- **Manual:** `POST /environments/{id}/deployments` with a build job ID or a release ID (developer role or higher).
- **Automatic:** an environment with `auto_deploy = true` deploys every succeeded build of its application. It defaults to **false**, so production changes only when someone opts in; staging environments typically turn it on.

  The deployment is created in the same transaction that records the build's success. It is skipped for builds of an **explicit commit** (`trigger: commit`, such as rebuilding an old SHA), and for a build **older than what the environment already deploys or has queued**, because builds can finish out of order and a slow old build must not replace a newer one. A problem in one environment (for example an invalid stored configuration) is logged and skipped; it does not fail the build or the other environments.

### Status and stages

Deployment status: `queued → running → succeeded | failed | cancelled`. While running it reports a stage, shown in the UI as `Queued → Pulling → Starting → Health check → Healthy`: `pull`, `start`, `health_check`.

### Failure and automatic rollback

If the new service does not pass its health check, the Agent captures the new container's last log lines into the job log, then **restores the previous release's image and configuration** (when there is one) and checks that it is healthy. The deployment is `failed` with `health_check_failed`; the environment keeps running its previous release. With no previous release, the failed service is left stopped for inspection. Other failures (pull, start) leave the previous service untouched where possible. Cancelling a running deployment is not supported in the MVP.

## 2. Service configuration (4.2)

An environment's service is described by **structured fields, not a Compose file**. A user-supplied Compose file can mount the host, run privileged containers, or join the host network, which would give a project member control of the server. The Control Plane validates the fields and the Agent renders the Compose file itself, so unsafe options cannot be expressed.

### Fields

| Field | Rules |
|---|---|
| `container_port` | Optional, 1–65535. The port the application listens on inside the container. |
| `published_port` | Optional, 1–65535, requires `container_port`. The host port. Unique per server across environments (database constraint). Ports below 1024 are rejected. |
| `bind_address` | `127.0.0.1` (default) or `0.0.0.0`. Services are not exposed to the network unless this is set explicitly, so a reverse proxy on the host is the default front door. |
| `env` | Map of plain environment variables: names `[A-Za-z_][A-Za-z0-9_]*`, at most 64 entries, 4 KB per value. Not for secrets; secret references arrive in Phase 5 and are never logged. |
| `volumes` | List of `{name, path}`: a **named** Docker volume mounted at an absolute container path. Host paths, `docker.sock`, and bind mounts are not accepted. |
| `restart` | `unless-stopped` (default), `always`, `on-failure`, or `no`. |
| `memory_mb`, `cpus` | Optional limits. |
| `health_check` | See below. |

Not configurable, by design: `privileged`, `network_mode: host`, capabilities, devices, `pid`/`ipc` modes, extra mounts, `command`/`entrypoint` overrides. The application is one service per environment in the MVP.

### Health check

| `type` | Passes when |
|---|---|
| `http` | `GET http://127.0.0.1:<published_port><path>` returns 2xx/3xx. Requires `published_port`. |
| `tcp` | A TCP connection to `<published_port>` succeeds. Requires `published_port`. |
| `docker` | The image's own `HEALTHCHECK` reports `healthy`. |

Plus `timeout_seconds` (default 60, max 600) and `interval_seconds` (default 2). The check runs from the Agent on the server, polls until it passes or the timeout is reached, and requires the container to stay running. A container that is merely running is not healthy. The default is `docker` when no published port is set and `tcp` otherwise.

### Rendering and applying

The file is **JSON** (valid YAML, which Compose reads), produced by the encoder, so no value can break out of its string; `$` in values is escaped as `$$` so Compose does not interpolate it. For each environment the Agent keeps `compose.yml` under its state directory, with Compose project `forgedeck-<environment id>` and a single service `app`. It carries the labels `forgedeck.managed` and `forgedeck.environment` plus Compose's own, so the dashboard's container list shows it. There is deliberately no per-deployment label: Compose recreates a container whose configuration changed, and a label that differs on every deployment would restart an unchanged service each time. Which deployment a container belongs to is recorded by the Control Plane.

Applying a deployment is idempotent: render, `docker compose pull`, `docker compose up -d --remove-orphans --no-build --pull never`, then run the health check. Re-applying an unchanged configuration leaves the container running; a changed image or configuration recreates it. The Agent needs the Docker Compose plugin on the host (documented as a requirement; the Agent reports it missing as `compose_unavailable`).

### Persistent volumes and downtime

- **Downtime:** the service is recreated, so there is a short gap while the old container stops and the new one starts. Blue/green and canary are out of scope for the MVP.
- **Volumes:** named volumes survive recreation. The Agent never runs `down -v` or removes volumes; deleting an environment leaves its volumes on the server and says so.
- **Rollback and data:** rolling back restores the old image and configuration, not the data. A new version that migrated a volume or database schema may not run on the old image. This risk is documented in 4.18 and shown before a rollback is confirmed.

### Registry access for pulls

The Agent pulls with credentials the Control Plane adds to the claim response of a `deploy_image` job ([05 §8](05-agent-protocol.md)). They are scoped to the one registry that hosts the image, built per attempt, never stored in the job payload or logs, and written to a per-job Docker config that is deleted afterwards.
