# 05 — Agent Protocol

## 1. Purpose

The ForgeDeck Agent runs on a target Docker server and executes a narrow set of approved operations. It initiates outbound HTTPS connections to the Control Plane so the target server does not need an inbound public port.

## 2. Responsibilities

- Enroll with a short-lived enrollment token.
- Obtain and protect its own identity/credential.
- Send periodic heartbeat and version/status information.
- Claim only jobs authorized for its registered server.
- Execute structured Docker operations.
- Report status, logs, and completion.
- Reconnect after temporary network failure.
- Stop accepting work after revocation.

## 3. Non-responsibilities

- It is not the CI build Runner.
- It does not run arbitrary commands supplied by an untrusted browser user.
- It does not expose Docker Engine over the network.
- It does not decide project permissions; the Control Plane authorizes jobs.

## 4. Enrollment lifecycle

1. An authorized project member creates a short-lived, single-use enrollment token. The token is bound to that project.
2. The token is delivered securely to the server operator.
3. The Agent submits the token over HTTPS.
4. The Control Plane validates the token, creates a server record in the token's project, and binds the Agent to it.
5. The Agent receives or establishes a unique credential.
6. The raw enrollment token is invalidated.
7. The Agent stores its credential with restrictive file permissions or an OS-supported secret store.
8. The server appears as online after a successful heartbeat.

Tokens should expire, be single-use, and be revocable. Avoid placing them in shell history or logs.

## 5. Heartbeat

Suggested initial interval: 15–30 seconds, with jitter to avoid synchronized requests.

Heartbeat data may include:
- Agent ID and version.
- Server status.
- Timestamp.
- Limited resource summary.
- Docker availability.
- Capability/version information.

The Control Plane should compute online/offline status based on last successful heartbeat and a configurable timeout. Metrics reported by the Agent are operational hints, not security evidence.

Heartbeats stay small. Container state is sent separately as a snapshot.

## 6. Container snapshot

The Agent reports the containers on its Docker Engine with `POST /agent/containers/snapshot`. The Control Plane stores the latest snapshot in `server_containers`, and the dashboard reads from there instead of calling the Agent synchronously.

The Agent sends a snapshot:
- On startup and after reconnecting.
- After completing any job that can change container state (for example `container_action`).
- Periodically at a configurable interval (suggested 30–60 seconds), and may skip sending if nothing changed since the last accepted snapshot.

Each snapshot is a full list for that server (not a diff) and includes `observed_at`, the time the Agent read state from Docker:

```json
{
  "observed_at": "2026-10-13T01:20:05Z",
  "containers": [
    {
      "container_id": "3f2a...",
      "name": "api",
      "image": "ghcr.io/example/api@sha256:...",
      "state": "running",
      "status_text": "Up 2 hours",
      "labels": { "forgedeck.application_id": "uuid" }
    }
  ]
}
```

Bound the snapshot size (number of containers and label size). Do not include environment variables, mounts, or other inspect data that may contain secrets.

The UI must show the snapshot's age from `observed_at` and mark it stale when it exceeds a threshold or the server is offline.

## 7. Job claiming

The Agent claims work with a long-poll on `POST /agent/jobs/claim`. The Control Plane holds the request until a job targeted at that Agent's server is queued or the wait limit (suggested 25 seconds) is reached, then returns the job or `204 No Content`. The Agent sends the next claim right away; on errors it backs off with jitter.

The Control Plane selects only jobs where `worker_kind = 'agent'` and `target_server_id` matches the server bound to the authenticated Agent credential. The Agent never chooses which server it claims for.

Long-poll lowers pickup latency compared with fixed-interval polling, but the end-to-end target (action visible in the UI within 1–3 seconds on a healthy connection) must be measured, not assumed. Record claim wait time, job pickup latency, and action-to-snapshot latency.

An Agent may run one job at a time in MVP. Concurrency can be added later with a per-Agent limit.

A claim response should include:
- Job ID.
- Job type.
- Lease duration/expiry.
- Protocol version.
- Structured, validated payload.
- Any short-lived, narrowly scoped credentials required for the operation.

Do not send general provider tokens or unrelated project secrets to an Agent.

## 8. Job payloads

Prefer typed payloads. Phase 1 needs two Agent job types.

`container_action`:

```json
{
  "type": "container_action",
  "container_id": "3f2a...",
  "action": "restart"
}
```

- `action` is one of `start`, `stop`, `restart`.
- The operation must be idempotent: `start` on a running container or `stop` on a stopped one succeeds without change.
- If the container no longer exists, fail with a clear error code (for example `container_not_found`).
- After completion, send a fresh container snapshot.
- The Agent should refuse actions on its own container, if it runs in one.

`fetch_logs`:

```json
{
  "type": "fetch_logs",
  "container_id": "3f2a...",
  "tail_lines": 500,
  "since": "2026-10-13T00:00:00Z",
  "timestamps": true
}
```

- Returns a bounded range of past logs through job events, stored as `job_log_chunks`.
- The Agent enforces its own maximum lines and bytes even if the payload asks for more, and reports truncation.
- Apply secret redaction before sending.
- Live tail is out of scope for Phase 1.

Later phases add deployment jobs such as:

```json
{
  "type": "deploy_image",
  "application_id": "uuid",
  "environment_id": "uuid",
  "image": "registry.example/app",
  "digest": "sha256:...",
  "health_check": {
    "path": "/health",
    "timeout_seconds": 30
  }
}
```

This is illustrative. Define a versioned schema before implementation. Do not include arbitrary shell command strings as the normal job interface.

## 9. Lease and heartbeat

While a job runs, the Agent renews its lease. If the Agent loses connectivity, it should attempt to reconnect and report the job state. If the lease expires, the Control Plane may requeue the job according to the job type's retry policy.

A lease is not a guarantee that the previous process has stopped. Design operations to be idempotent and handle duplicate attempts.

## 10. Logs and events

- Assign monotonically increasing sequence numbers per attempt.
- Send logs in bounded batches.
- Include event type and timestamp.
- Redact known secrets before transmitting.
- Apply backpressure; do not buffer unlimited logs in memory.
- Support truncation/retention policy.
- Report status transitions separately from free-form log lines.

## 11. Revocation and rotation

When an Agent is revoked:
- Reject new job claims and authenticated requests.
- Stop trusting its future heartbeat.
- Provide a documented credential rotation/re-enrollment path.
- Record the action in the audit log.

A disconnected Agent cannot receive a revocation message immediately, so jobs should use short leases and sensitive operations should revalidate authorization as appropriate. A pending long-poll claim from a revoked Agent must be ended without returning a job.

## 12. Docker operation safety

The Agent may support a limited allowlist such as list, inspect, logs, start, stop, restart, and deployment of a validated image/configuration. Enforce operation-specific authorization and resource constraints.

Never accept arbitrary Docker API paths from the browser. Do not expose an unauthenticated Docker TCP socket.

## 13. Compatibility

Include a protocol version. The Control Plane should reject unsupported incompatible versions with a clear error. Define how Agent upgrades are delivered and rolled back before enabling automatic upgrades.

## 14. Decisions

- Job claims use long-poll (section 7).
- Container list comes from Agent snapshots stored by the Control Plane (section 6).
- Container logs in Phase 1 are fetched as bounded `fetch_logs` jobs; live tail is deferred.

## 15. Open decisions

- Whether log events use HTTP batches or a streaming connection.
- Mechanism for live tail of container logs.
- Credential format and rotation mechanism.
- Agent package distribution and supported Linux platforms.
- How to apply Compose updates safely when persistent volumes are present.
