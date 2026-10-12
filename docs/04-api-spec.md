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

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/git-connections` | List connections available to user |
| POST | `/git-connections` | Create provider connection |
| DELETE | `/git-connections/{id}` | Revoke/remove connection |
| GET | `/git-connections/{id}/repositories` | List accessible repositories |
| GET | `/projects/{projectID}/applications` | List applications |
| POST | `/projects/{projectID}/applications` | Create application |
| GET | `/applications/{applicationID}` | Get application |
| PATCH | `/applications/{applicationID}` | Update application |
| DELETE | `/applications/{applicationID}` | Delete application after safeguards |

## 6. Servers and containers

Servers belong to a project. Every server-scoped endpoint resolves the project through `servers.project_id` and checks the caller's membership and role.

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/servers` | List servers across projects the caller can access (Overview) |
| GET | `/projects/{projectID}/servers` | List servers in a project |
| POST | `/projects/{projectID}/servers/enrollment-tokens` | Create short-lived enrollment token bound to the project |
| GET | `/servers/{serverID}` | Server detail |
| DELETE | `/servers/{serverID}` | Remove/revoke server access |
| GET | `/servers/{serverID}/containers` | List containers from the latest snapshot |
| POST | `/servers/{serverID}/containers/{containerID}/actions` | Queue start/stop/restart (`container_action` job) |
| POST | `/servers/{serverID}/containers/{containerID}/logs` | Queue a bounded log fetch (`fetch_logs` job) |

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
| GET | `/jobs` | Paginated job history |
| GET | `/jobs/{jobID}` | Job detail |
| GET | `/jobs/{jobID}/logs` | Paginated stored log chunks (`?after_sequence=`) |
| GET | `/jobs/{jobID}/events` | SSE stream for live logs/events |
| POST | `/jobs/{jobID}/cancel` | Request cancellation where supported |
| POST | `/applications/{applicationID}/builds` | Start build |
| GET | `/environments/{environmentID}/deployments` | List deployments |
| POST | `/environments/{environmentID}/deployments` | Deploy selected commit/image |
| GET | `/environments/{environmentID}/releases` | List releases |
| POST | `/releases/{releaseID}/rollback` | Roll back to a previous release |

A deployment request should specify a validated commit or previously built image, target environment, and optional safe configuration overrides. Do not accept arbitrary shell commands as a deployment payload.

## 8. Webhooks

- `POST /webhooks/github`
- `POST /webhooks/gitlab`

Webhook handlers should verify signatures/secrets, validate event type and repository association, deduplicate delivery IDs, and enqueue work quickly. Long-running builds must not run inside the webhook request.

## 9. Agent endpoints

These endpoints are machine-to-machine and require Agent authentication:

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/agent/enroll` | Exchange enrollment token for Agent credential |
| POST | `/agent/heartbeat` | Liveness, version, resource summary |
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

SSE handlers should support reconnect/resume where practical, using a sequence/event ID. Apply authorization before streaming and stop streams when the client disconnects.

## 11. API quality requirements

- Consistent pagination, filtering, and sort behavior.
- Stable error codes.
- Request IDs for troubleshooting.
- Rate limiting for login, enrollment, and sensitive endpoints.
- Explicit timeouts and bounded request sizes.
- OpenAPI documentation generated or maintained with the API.
- Contract tests for endpoints used by the Web UI and Agent.
