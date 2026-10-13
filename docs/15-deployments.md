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

### Status and stages

Deployment status: `queued → running → succeeded | failed | cancelled`. While running it reports a stage, shown in the UI as `Queued → Pulling → Starting → Health check → Healthy`: `pull`, `start`, `health_check`.

### Failure and automatic rollback

If the new service does not pass its health check, the Agent captures the new container's last log lines into the job log, then **restores the previous release's image and configuration** (when there is one) and checks that it is healthy. The deployment is `failed` with `health_check_failed`; the environment keeps running its previous release. With no previous release, the failed service is left stopped for inspection. Other failures (pull, start) leave the previous service untouched where possible. Cancelling a running deployment is not supported in the MVP.
