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

### Rollback

`POST /releases/{id}/rollback` (developer or higher) creates a deployment whose source is that release: the **same image digest and the configuration the release ran with**, never a rebuild of old source. The deployment follows the normal flow, including the health check and, if the old version no longer starts, the fall back to what ran before. When it is healthy it becomes the environment's newest release, so history keeps every release and nothing is rewritten. Any release can be chosen, including the current one (which re-applies it).

The environment's own configuration is not changed, so later deployments keep using what the user set there. Before queuing, the Control Plane refuses a rollback whose host port another environment on the server has taken since (`409 port_in_use`). It does not check that the image is still in the registry or that volumes still match the old version: a missing image fails the pull (the running service stays up), and data compatibility is the risk documented in 4.18.

### Triggers

- **Manual:** `POST /environments/{id}/deployments` with a build job ID or a release ID (developer role or higher).
- **Automatic:** an environment with `auto_deploy = true` deploys every succeeded build of its application. It defaults to **false**, so production changes only when someone opts in; staging environments typically turn it on.

  The deployment is created in the same transaction that records the build's success. It is skipped for builds of an **explicit commit** (`trigger: commit`, such as rebuilding an old SHA), and for a build **older than what the environment already deploys or has queued**, because builds can finish out of order and a slow old build must not replace a newer one. A problem in one environment (for example an invalid stored configuration) is logged and skipped; it does not fail the build or the other environments.

### Status and stages

Deployment status: `queued → running → succeeded | failed | cancelled`. While running it reports a stage, shown in the UI as `Queued → Pulling → Starting → Health check → Healthy`: `pull`, `start`, `health_check`.

### Failure and automatic rollback

If the new service does not pass its health check, the Agent captures the new container's last log lines into the job log, then **restores the previous release's image and configuration** (when there is one, taken from the claim, see [05 §8](05-agent-protocol.md)) and checks that it is healthy. Restoring tolerates a failed pull because the image is normally still on the host. A previous configuration that no longer validates is not restored. The deployment is `failed` with `health_check_failed`, and its message says which state the environment is in: the previous release restored and running, restoring failed too (the service may be down), or no previous release (the failed service was stopped so it does not crash-loop). Other failures (pull, start) leave the previous service untouched where possible. Cancelling a running deployment is not supported in the MVP.

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

Plus `timeout_seconds` (default 60, max 600) and `interval_seconds` (default 2). The check runs from the Agent on the server (so the Agent must run on the host, or in a container that shares the host network; otherwise use `docker`). It polls until the check has **passed twice in a row** (one lucky answer between restarts of a crash-looping container is not health) or the timeout is reached, and fails at once if the container stops running or restarts. A container that is merely running is not healthy. A `docker` check needs an image `HEALTHCHECK` and says so when there is none. The default is `docker` when no published port is set and `tcp` otherwise.

When the check fails, the Agent writes the last 50 lines of the container's output to the job log. All job logs have the registry password and the configuration's environment values masked.

### Rendering and applying

The file is **JSON** (valid YAML, which Compose reads), produced by the encoder, so no value can break out of its string; `$` in values is escaped as `$$` so Compose does not interpolate it. For each environment the Agent keeps `compose.yml` under its state directory, with Compose project `forgedeck-<environment id>` and a single service `app`. It carries the labels `forgedeck.managed` and `forgedeck.environment` plus Compose's own, so the dashboard's container list shows it. There is deliberately no per-deployment label: Compose recreates a container whose configuration changed, and a label that differs on every deployment would restart an unchanged service each time. Which deployment a container belongs to is recorded by the Control Plane.

Applying a deployment is idempotent: render, `docker compose pull`, `docker compose up -d --remove-orphans --no-build --pull never`, then run the health check. Re-applying an unchanged configuration leaves the container running; a changed image or configuration recreates it. The Agent needs the Docker Compose plugin on the host (documented as a requirement; the Agent reports it missing as `compose_unavailable`).

### Persistent volumes and downtime

- **Downtime:** the service is recreated, so there is a short gap while the old container stops and the new one starts. Blue/green and canary are out of scope for the MVP.
- **Volumes:** named volumes survive recreation. The Agent never runs `down -v` or removes volumes; deleting an environment leaves its volumes on the server and says so.
- **Rollback and data:** rolling back restores the old image and configuration, not the data. A new version that migrated a volume or database schema may not run on the old image. This risk is documented in 4.18 and shown before a rollback is confirmed.

### Registry access for pulls

The Agent pulls with credentials the Control Plane adds to the claim response of a `deploy_image` job ([05 §8](05-agent-protocol.md)). They are scoped to the one registry that hosts the image, built per attempt, never stored in the job payload or logs, and written to a per-job Docker config that is deleted afterwards.

## 3. Risks: volumes and database migrations (4.18)

A rollback changes **code and configuration, not data**. Everything below follows from that.

### What a rollback does and does not undo

| Restored by a rollback | Not restored |
|---|---|
| The image (by digest) | Files a newer version wrote to a **named volume** |
| The service configuration snapshot (ports, environment variables, volume mounts, restart policy, health check) | Changes a newer version made to an **external database** (migrations, new rows) |
| | Anything the newer version sent to other systems |

If version N+1 migrated a schema or rewrote a data file, version N may fail to start, start and misbehave, or corrupt the new data. ForgeDeck cannot tell which; the health check only proves the old version answers, not that it reads its data correctly.

### Writing releases that can be rolled back

- **Expand, then contract.** Ship schema changes in two releases: first one that works with both the old and the new schema (add columns and tables, write both, read the old), later one that drops what nothing uses any more. Never rename or drop in the same release that stops using the old shape. This is what makes "roll back one release" safe.
- Make migrations **backward compatible with the previous release**. Before relying on a rollback, ask whether the previous image can run on the schema as it is now.
- Prefer **application-level compatibility** (the old code ignores new columns) to down-migrations. Down-migrations run on production data that has changed since they were written.
- Do not run destructive migrations automatically at container start. Run them as a deliberate step after the release has proven healthy.

### Back up before changing data

ForgeDeck does not back up volumes or databases (see Phase 5 for the Control Plane's own backup). Before a release that changes data, take a backup you have tested restoring:

- A volume: `docker run --rm -v <volume>:/data -v "$PWD":/backup busybox tar czf /backup/<volume>.tgz -C /data .` (the volume's real name is `forgedeck-<environment id>_<volume name>`).
- A database: the database's own dump tool, to a place that is not the volume it protects.

A rollback after a bad migration is then a restore, not a redeploy.

### Volumes

- Deployments recreate the container but keep named volumes. Nothing in ForgeDeck deletes a volume: not a rollback, not a failed deployment, not deleting the environment (which stops managing the service but leaves it running with its data).
- To remove a service and its data on purpose, on the server: `docker compose --project-name forgedeck-<environment id> down --volumes`.
- **Two containers must not share one volume** unless the application supports it. A deployment replaces the old container before starting the new one, so the two do not overlap; do not mount the same volume into a second environment.
- A configuration restored by a rollback mounts the volumes it listed then. If a newer release added a volume, the rolled-back service no longer mounts it (the volume stays on the server).

### Downtime

A deployment stops the old container and starts the new one, so the service is down for a few seconds to the length of its start-up. If the new version fails its health check, the previous release is restored, which adds the same gap again. Zero-downtime strategies (blue/green, canary) are out of scope for the MVP, as is running several replicas of one environment.

### Secrets and configuration

Environment variables are stored in the environment's configuration and in each release's snapshot, and are visible to project members. They are not for secrets. A rollback restores the variables the old release had, which can include a value that has since been rotated. Secret references arrive in Phase 5.

## 4. Project secrets (Phase 5)

Environment variables in a configuration are visible to every project member and are copied into each release's snapshot, so they are not for secrets. A **project secret** is a named value that is stored encrypted, is write-only, and is injected into a service at deployment time.

### Model

- A secret belongs to a project: a `name` (`[A-Z_][A-Z0-9_]*`, at most 64 characters) and a `value` (6 bytes to 4 KB). The value is sealed with the keyring (AES-256-GCM, bound to the secret's row) like Git tokens and registry passwords.
- It is **write-only**: the API lists names with `version` and `updated_at`, and never returns a value. Setting a secret again replaces the value and increments `version`.
- An environment uses a secret by listing its name in `config.secrets`. The service then sees an environment variable of that name. A secret name may not also appear in `config.env`. The environment form checks that every listed secret exists.
- **Releases store references, not values.** A release's snapshot holds the secret names. A rollback restores the old image and configuration and then uses the **current** value of each secret: rotating a secret is never undone by a rollback, and no old secret value is kept anywhere.
- A change to a secret takes effect on the **next deployment** of each environment that uses it; nothing restarts on its own.
- A secret that an environment lists cannot be deleted (`409 secret_in_use`); remove it from the environment first.
- Setting, replacing, or deleting a secret requires the admin role (the same as managing applications). Members can list names.

### Delivery and handling

- The Control Plane decrypts the secrets of the claimed deployment (and of the previous release it may restore) when the Agent claims the job and adds them to the claim response, like registry credentials ([05 §8](05-agent-protocol.md)). They are never in the job payload, a release, or a log.
- The Agent passes them to `docker compose` in the **process environment** and the Compose file only refers to them (`NAME: ${NAME}`), so no secret value is written to disk by ForgeDeck. The running container's configuration still holds the value, as it does for any Docker environment variable: anyone who can run `docker inspect` on the server can read it. Treat access to the Docker host as access to the secrets.
- The Agent masks the values in the job log. The Control Plane also masks every project secret in log lines it receives (defense in depth, including container logs fetched for a server); secrets shorter than 6 bytes cannot be created because they could not be masked reliably.
- If a listed secret no longer exists when the job is claimed, the attempt fails at once with `secret_not_found`.
