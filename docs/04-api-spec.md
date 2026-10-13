# 04 — API Specification

## 1. Conventions

Base path: `/api/v1`

- JSON request/response bodies unless the endpoint is an SSE stream.
- Use UTC timestamps in ISO 8601 format.
- Authenticate browser requests with secure sessions.
- Authorize every resource access server-side.
- Paginate list endpoints.
- Validate request sizes and fields.
- Do not return secret values after creation.
- API examples are illustrative and must be finalized alongside implementation.

## 2. Suggested error format

```json
{
  "error": {
    "code": "deployment_health_check_failed",
    "message": "The application did not become healthy.",
    "request_id": "req_..."
  }
}
```

Do not expose stack traces, credentials, or internal host details in user-facing errors.

## 3. Authentication

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/auth/login` | Start a session |
| POST | `/auth/logout` | End a session |
| GET | `/auth/me` | Return current user |

Use CSRF protection appropriate to the chosen cookie/session approach. Cookies should be `HttpOnly`, `Secure` in HTTPS deployments, and use an appropriate `SameSite` policy.

## 4. Projects

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/projects` | List accessible projects |
| POST | `/projects` | Create project |
| GET | `/projects/{projectID}` | Get project |
| PATCH | `/projects/{projectID}` | Update project |
| DELETE | `/projects/{projectID}` | Delete project after safeguards |
| GET | `/projects/{projectID}/members` | List members |
| POST | `/projects/{projectID}/members` | Add an existing user by email with a role |
| DELETE | `/projects/{projectID}/members/{userID}` | Remove member (cannot remove a higher role or the last owner) |

Members are managed by Admin and Owner roles. Nobody can grant or remove a role higher than their own, and any member may remove themselves unless they are the last owner.

## 5. Git connections and applications

See [13 — Git Providers](13-git-providers.md) for provider setup.

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/projects/{projectID}/git-connections` | List the project's connections |
| POST | `/projects/{projectID}/git-connections/github` | Start a GitHub App install; returns `install_url` |
| GET | `/git/github/setup` | GitHub install callback (no session; verified by sealed state and OAuth) |
| POST | `/projects/{projectID}/git-connections/gitlab` | Connect GitLab with `base_url` and an access token |
| DELETE | `/git-connections/{connectionID}` | Remove a connection and its stored credentials |
| GET | `/git-connections/{connectionID}/repositories` | Repositories the connection can read (admin) |
| GET | `/projects/{projectID}/applications` | List applications |
| POST | `/projects/{projectID}/applications` | Create application |
| GET | `/applications/{applicationID}` | Get application |
| PATCH | `/applications/{applicationID}` | Update application |
| DELETE | `/applications/{applicationID}` | Delete application after safeguards |
| GET | `/projects/{projectID}/registries` | List container registries |
| POST | `/projects/{projectID}/registries` | Add a registry: `name`, `host`, `username`, `password` (admin) |
| DELETE | `/registries/{registryID}` | Remove a registry (`409 registry_in_use` while applications push to it) |

An application's image destination is `registry_connection_id` plus `image_repository` (e.g. `acme/shop-api`, no host or tag), set together on create or update; `"image_repository": ""` removes both. The registry must belong to the application's project. Registry passwords are write-only.

### Project secrets

See [15 — Deployments §4](15-deployments.md#4-project-secrets-phase-5).

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/projects/{projectID}/secrets` | List secrets: `name`, `version`, timestamps; never a value |
| PUT | `/projects/{projectID}/secrets/{name}` | Create (`201`) or replace (`200`, `version` + 1) with `{ "value": "…" }` (admin) |
| DELETE | `/projects/{projectID}/secrets/{name}` | Delete (`409 secret_in_use` while an environment lists it) (admin) |

`name` is `[A-Z_][A-Z0-9_]{0,63}`; the value is 6 bytes to 4 KB. A secret is used by listing its name in an environment's `config.secrets` (the environment API rejects unknown names with `422`); the service receives it as an environment variable of that name at the next deployment.

### Audit log

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/projects/{projectID}/audit-logs` | Events newest first; `?action=` filters by action; keyset `cursor`; admin or owner only |

Each entry has `actor`, `action`, `target_type`, `target_id`, `target_name`, `metadata`, `request_id`, and `created_at`. See [03](03-database.md) for what is recorded and what is never put in metadata.

### Environments

See [15 — Deployments](15-deployments.md) for the model and the `config` fields.

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/applications/{applicationID}/environments` | List environments |
| POST | `/applications/{applicationID}/environments` | Create: `name`, `server_id`, optional `auto_deploy`, optional `config` (admin) |
| GET | `/environments/{environmentID}` | Get |
| PATCH | `/environments/{environmentID}` | Change `name`, `server_id`, `auto_deploy`, or replace `config` (admin) |
| DELETE | `/environments/{environmentID}` | Delete; volumes on the server are left in place (admin) |

`config` is validated and returned with defaults filled (loopback bind, `unless-stopped`, a health check). The server must belong to the application's project, and a published port may be used once per server (`422` otherwise). A server with environments cannot be removed (`409 server_in_use`). Unknown `config` fields are rejected.

## 6. Servers and containers

Servers belong to a project. Every server-scoped endpoint resolves the project through `servers.project_id` and checks the caller's membership and role.

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/servers` | List servers across projects the caller can access (Overview) |
| GET | `/projects/{projectID}/servers` | List servers in a project |
| POST | `/projects/{projectID}/servers/enrollment-tokens` | Create short-lived enrollment token bound to the project |
| GET | `/servers/{serverID}` | Server detail |
| DELETE | `/servers/{serverID}` | Remove/revoke server access |
| POST | `/servers/{serverID}/credential-rotation` | Ask the server's Agent to rotate its credential at its next heartbeat (admin) |
| GET | `/servers/{serverID}/containers` | List containers from the latest snapshot |
| POST | `/servers/{serverID}/containers/{containerID}/actions` | Queue start/stop/restart (`container_action` job) |
| POST | `/servers/{serverID}/containers/{containerID}/logs` | Queue a bounded log fetch (`fetch_logs` job); developer or higher, as is reading the job's `/logs` and `/events` |

The server record is created when an Agent enrolls with a project-bound token; there is no separate server registration endpoint in MVP.

### Container list

The list is read from `server_containers`; it does not call the Agent synchronously.

```json
{
  "server_id": "uuid",
  "observed_at": "2026-10-13T01:20:05Z",
  "stale": false,
  "containers": [
    {
      "container_id": "3f2a...",
      "name": "api",
      "image": "ghcr.io/example/api@sha256:...",
      "state": "running",
      "status_text": "Up 2 hours"
    }
  ]
}
```

`stale` is `true` when `observed_at` is older than the configured threshold or the server is offline. The UI must show data age instead of presenting cached state as live.

### Container actions and log fetch

Both endpoints are asynchronous. They validate that `containerID` exists in the server's latest snapshot, create an Agent job with `target_server_id = serverID`, and return `202 Accepted`:

```json
{ "job_id": "uuid", "status": "queued" }
```

Action request body: `{ "action": "start" | "stop" | "restart" }`.

Log fetch request body (bounded by server-side maximums):

```json
{ "tail_lines": 500, "since": "2026-10-13T00:00:00Z", "timestamps": true }
```

The client follows the job with `GET /jobs/{jobID}` and reads output with `GET /jobs/{jobID}/logs`. Live tail of container logs is deferred.

Never accept a client-supplied Docker socket, host address, or Docker API path.

## 7. Jobs and deployments

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/jobs` | Job history, newest first; filters `project_id`, `application_id`, `server_id`, `status`, `type`; keyset `cursor` |
| GET | `/jobs/{jobID}` | Job detail |
| GET | `/jobs/{jobID}/logs` | Stored log lines after a keyset cursor (`?after=<attempt>.<sequence>&limit=`) |
| GET | `/jobs/{jobID}/events` | SSE stream for live logs/events |
| POST | `/jobs/{jobID}/cancel` | Request cancellation where supported |
| POST | `/applications/{applicationID}/builds` | Start build |
| GET | `/environments/{environmentID}/deployments` | List deployments |
| POST | `/environments/{environmentID}/deployments` | Deploy selected commit/image |
| GET | `/environments/{environmentID}/releases` | List releases |
| POST | `/releases/{releaseID}/rollback` | Roll back to a previous release |

`POST /applications/{applicationID}/builds` (developer or higher) takes an optional body `{ "commit_sha": "<40-char sha>" }`; without it, the head of the application's branch is built. The commit is resolved through the Git provider (`404` if it does not exist), the application must have a registry and image (`409 no_image_destination`), and if the same commit is already queued or building that job is returned instead of a new one. Response `202`:

```json
{ "job_id": "uuid", "status": "queued", "commit_sha": "0123…" }
```

Follow it like any job (`GET /jobs/{jobID}`, `/events`). A succeeded build's `result` holds `image`, `tag`, `digest`, and `image_ref` ([14](14-runner.md)).

`POST /environments/{environmentID}/deployments` (developer or higher) takes `{ "build_job_id": "<uuid>" }`: a **succeeded build of the same application**, whose result holds an image digest. The deployment uses the environment's current configuration and the image's digest, never a tag. It returns `202` with the deployment (`status: queued`) and queues a `deploy_image` job for the environment's server, in one transaction. `422` if the build is not usable (still running, failed, another application's, or without a digest). A deployment follows its job: `queued → running → succeeded | failed`, with `error.code` and `error.message` on failure. While it runs, `stage` is the newest of `pull`, `start`, `health_check` that the Agent reported (its log lines are tagged `[pull] …`); once it failed, `stage` is where, taken from the error code (`registry_login_failed`/`pull_failed` → `pull`, `start_failed` → `start`, `health_check_failed` → `health_check`). A deployment that passed its health check gets a `release_id`. `GET /deployments/{deploymentID}` and `GET /environments/{environmentID}/deployments` (paginated, newest first) read it. Environments with `auto_deploy` also get a deployment whenever a build succeeds ([15](15-deployments.md)); such a deployment has no `created_by`. `GET /environments/{environmentID}/releases` lists releases newest first (paginated); each has `commit_sha`, `image_ref`, the `config` snapshot it ran with, and `current: true` for the newest. A release is recorded in the same transaction that records the successful `deploy_image` job, so a release exists exactly when its deployment became healthy. `POST /releases/{releaseID}/rollback` (developer or higher) redeploys a release's digest and configuration snapshot as a new deployment (`202`, same shape as a deployment, with `source_release_id`). `409 port_in_use` if another environment on the server now publishes the release's port. See [15](15-deployments.md#rollback). Arbitrary shell commands are never accepted as a deployment payload.

### Overview

`GET /overview` summarizes every project the caller belongs to; any member (viewer and up) may read it, and nothing outside the caller's projects is counted or listed.

```json
{
  "servers": { "total": 4, "online": 2, "unreachable": 1, "offline": 1, "pending": 0 },
  "environments": { "total": 5, "succeeded": 3, "failed": 1, "in_progress": 1, "not_deployed": 0 },
  "recent_deployments": [ { "id": "uuid", "environment_id": "uuid", "environment_name": "staging", "application_id": "uuid", "application_name": "API", "project_name": "Shop", "commit_sha": "0123…", "status": "failed", "error_code": "health_check_failed", "created_at": "…", "finished_at": null } ],
  "failing_deployments": [ "…same shape…" ],
  "generated_at": "…"
}
```

Server counts use the same heartbeat windows as a server's `status`. Environments are grouped by how their **newest** deployment ended (`not_deployed`: none yet, or only cancelled), so `succeeded` means the last deployment passed its health check, not that the service is healthy right now. `recent_deployments` is the eight newest across projects; `failing_deployments` holds up to five environments whose newest deployment failed, so a later success removes an entry.

## 8. Webhooks

- `POST /webhooks/github`
- `POST /webhooks/gitlab`

Webhook handlers should verify signatures/secrets, validate event type and repository association, deduplicate delivery IDs, and enqueue work quickly. Long-running builds must not run inside the webhook request.

## 9. Agent endpoints

These endpoints are machine-to-machine and require Agent authentication:

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/agent/enroll` | Exchange enrollment token for Agent credential |
| POST | `/agent/heartbeat` | Liveness, version, resource summary; the response may ask the Agent to rotate its credential |
| POST | `/agent/rotate-credential` | Replace the Agent's credential; the old one works for a 15-minute grace period |
| POST | `/agent/containers/snapshot` | Replace the server's container snapshot |
| POST | `/agent/jobs/claim` | Long-poll for the next job targeted at this server |
| POST | `/agent/jobs/{jobID}/heartbeat` | Renew job lease |
| POST | `/agent/jobs/{jobID}/events` | Append log/status events in bounded batches |
| POST | `/agent/jobs/{jobID}/complete` | Report final result |

`POST /agent/jobs/claim` is a long-poll: the Control Plane holds the request until a job is available or the wait limit is reached (suggested 25 seconds, kept below common reverse-proxy idle timeouts), then returns `200` with a job or `204 No Content`. The Agent reconnects immediately after each response. Claims only return jobs with `worker_kind = 'agent'` and `target_server_id` equal to the server bound to the authenticated Agent. See [03 — Database](03-database.md#5-queue-transaction-requirements).

Every endpoint must verify the Agent is active (not revoked) and authorized for the referenced server/job. The exact batching format for log events is defined in [05 — Agent Protocol](05-agent-protocol.md).

## 10. SSE event format

Example:

```text
event: log
id: 42
data: {"sequence":42,"stream":"stdout","message":"Building image"}

event: status
data: {"status":"building"}
```

`GET /jobs/{jobID}/events` sends `status` events (the whole job, whenever its status changes), `log` events with id `<attempt>.<sequence>`, keepalive comments every 15 seconds, and a final `end` event once the job has finished and every line was sent. A reconnecting `EventSource` sends `Last-Event-ID` and resumes after that line; `?after=<attempt>.<sequence>` does the same for clients that cannot set headers. Streams stop when the client disconnects or the server shuts down.

## 11. API quality requirements

- Consistent pagination, filtering, and sort behavior.
- Stable error codes.
- Request IDs for troubleshooting.
- Rate limiting for login, enrollment, and sensitive endpoints.
- Explicit timeouts and bounded request sizes.
- OpenAPI documentation generated or maintained with the API.
- Contract tests for endpoints used by the Web UI and Agent.
