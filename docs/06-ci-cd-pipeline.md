# 06 — CI/CD Pipeline

## 1. Goal

Provide a predictable path from a Git commit to a healthy Docker deployment, with visible stages, logs, and a recorded release.

## 2. Standard pipeline

```text
Git push
  ↓
GitHub/GitLab webhook
  ↓
Verify signature + deduplicate event
  ↓
Resolve project/application/branch
  ↓
Create build job
  ↓
Isolated Runner checks out exact commit SHA
  ↓
Run configured tests
  ↓
Build image with BuildKit/Buildx
  ↓
Push image to registry
  ↓
Record image digest
  ↓
Create deployment job
  ↓
Agent pulls image and updates Compose workload
  ↓
Health check
  ↓
Record release as healthy or failed
```

## 3. Webhook handling

Webhook request handling should be short:
1. Validate provider signature/token.
2. Validate event type and repository association.
3. Deduplicate using the provider delivery ID.
4. Determine whether the branch/event matches the application trigger rules.
5. Persist event and enqueue a job transactionally where possible.
6. Return a successful response without waiting for the build.

Do not execute repository code inside the webhook handler.

## 4. Build job

A build job should pin:
- Repository identity.
- Commit SHA.
- Build context.
- Dockerfile path.
- Allowed build settings.
- Registry destination.
- Timeout and resource limits.

The Runner must check out the exact commit SHA, not a moving branch name. It should use a temporary workspace, enforce limits, redact secrets, and clean up after completion.

## 5. Runner isolation

Repository code and package install scripts are untrusted. Prefer a dedicated build host or strong isolation boundary. Do not mount the production Docker socket into build jobs. Restrict resources, time, network access where feasible, and filesystem access.

Build credentials should be short-lived or scoped to the minimum repository/registry permissions needed.

## 6. Image identity

Use a readable tag for discoverability, for example a commit SHA tag, but record and deploy the resolved immutable image digest.

Example:

```text
registry.example/team/api:abc1234
registry.example/team/api@sha256:...
```

A mutable tag alone is not a sufficient release identity.

## 7. Deployment job

A deployment job should include:
- Application and environment IDs.
- Target server ID.
- Immutable image digest.
- Validated Compose/service configuration.
- Secret references (not plaintext values in logs).
- Health-check policy.
- Job/release correlation IDs.

The Agent pulls the image and applies the approved configuration. The exact update strategy must account for persistent volumes and service downtime.

## 8. Health check

After the container is started:
1. Confirm the expected container/service exists.
2. Wait for the configured health check.
3. Apply a bounded timeout and retry count.
4. Record result and relevant sanitized diagnostics.
5. Mark the release healthy only after the check passes.

A running container is not automatically a healthy application.

## 9. Release and rollback

A release record should include commit SHA, image reference/digest, environment, deployment job, timestamps, and status. Rollback should redeploy a previous known-good digest rather than rebuild old source.

Before rollback, validate that the image is still available and the old configuration/secrets remain compatible. Database schema changes may not be reversible simply by rolling back an application image.

## 10. Job states

Recommended general states:
- `queued`
- `running`
- `succeeded`
- `failed`
- `cancelled`

Pipeline stages can be represented as events or a stage field:
- `checkout`
- `test`
- `build`
- `push`
- `deploy`
- `health_check`

Keep job state transitions valid and auditable. Cancellation may not be possible for every stage; communicate that clearly.

Retry policy (Phase 2): a job is retried only when its worker is lost (the lease expires). A lost attempt may still have done its work, so attempts depend on whether repeating is safe:

| Job type | Attempts | Why |
|---|---|---|
| `container_action` | 1 | A restart that looked lost may have happened; repeating could restart twice |
| `fetch_logs` | 2 | Read-only |
| `build` | 2 | Pinned to one commit and keyed by it, so rebuilding is safe |

A job that fails on its own (tests fail, Docker error) is not retried automatically.

## 11. Failure behavior

- Checkout failure: report repository/commit context without leaking credentials.
- Test failure: preserve bounded test output.
- Build failure: expose the failing stage and relevant logs.
- Push failure: avoid marking the image as available.
- Agent unavailable: keep deployment queued or fail according to a clear timeout policy.
- Health-check failure: mark deployment failed and offer logs/retry/manual rollback.
- Lease expiry: recover the job without assuming the previous attempt definitely stopped.

## 12. MVP pipeline configuration

Start with a simple form:
- Repository and branch.
- Dockerfile path.
- Build context.
- Optional test command or a small set of predefined build steps.
- Registry connection.
- Target environment.
- Health-check path and timeout.

Avoid building a general-purpose CI YAML language in the first version.

## 13. Open decisions

- Whether deploy starts automatically after a successful build or requires approval in production.
- How Compose files are provided, validated, and versioned.
- Whether to support multiple services in one application in MVP.
- Rollback policy after health-check failure.
- Registry credential delivery and expiry behavior.
