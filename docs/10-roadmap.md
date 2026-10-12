# 10 — Roadmap and Acceptance Criteria

## Guiding rule

Build a small working vertical slice before expanding the feature set. Each milestone should have a clear acceptance test and should leave the system in a usable state.

## Phase 0 — Product and contracts

Deliver:
- Product scope agreed.
- Architecture and trust boundaries documented.
- Initial database model reviewed.
- API and Agent protocol contracts drafted.
- MVP exclusions explicit.

Acceptance:
- No unresolved design ambiguity blocks the first vertical slice.
- Documentation clearly labels open decisions.

## Phase 1 — Foundation and remote Docker visibility

Build:
- Go API and configuration.
- PostgreSQL migrations and data access.
- Initial authentication.
- Projects and basic authorization.
- Agent enrollment with project-bound tokens (servers belong to a project).
- Heartbeat and online/offline state.
- Container snapshots stored in `server_containers` with `observed_at`.
- Minimal Agent job queue: `worker_kind`/`target_server_id`, `SKIP LOCKED` claim, long-poll, lease, complete.
- `container_action` jobs for start/stop/restart behind authorization.
- `fetch_logs` jobs for bounded past container logs (no live tail).
- Minimal React/HeroUI shell embedded in and served by the API binary.

Acceptance:
- A test server can enroll and appear online in the token's project.
- The dashboard lists containers from the latest snapshot and shows its age; the list is marked stale when the snapshot is old or the server is offline.
- Start/stop/restart reach the Agent through a job, and the new state appears in a fresh snapshot.
- A job targeted at server A cannot be claimed by the Agent of server B.
- Log fetch returns a bounded range and reports truncation.
- Revoking the Agent blocks subsequent authenticated requests, including a pending long-poll claim.
- Unauthorized users cannot list, operate, or fetch logs from another project's server.
- Action-to-visible-state latency is measured and recorded under a documented setup (target 1–3 seconds on a healthy connection).
- Control Plane and Agent errors are visible and actionable.

## Phase 2 — Git integration and job queue

Build:
- GitHub and GitLab connection flow.
- Webhook signature verification and deduplication.
- Extend the Phase 1 job queue to the `runner` worker kind.
- Lease-expiry recovery, attempts, retry policy, cancellation semantics.
- Job history and SSE logs.

Acceptance:
- Duplicate webhook deliveries do not create duplicate builds.
- Concurrent workers do not claim the same queued job in the same attempt.
- Expired leases are recoverable.
- Logs arrive in order and are bounded.

## Phase 3 — Isolated CI Runner

Build:
- Runner registration/configuration.
- Checkout exact commit SHA.
- Test execution.
- BuildKit/Buildx image build.
- Registry push.
- Image digest capture.
- Timeouts, resource limits, workspace cleanup.

Acceptance:
- A known test repository can build successfully.
- A failing test produces a failed job with useful logs.
- The Runner does not use the production Docker socket.
- The resulting image digest is recorded.

## Phase 4 — Deployment and releases

Build:
- Applications and environments.
- Target server assignment.
- Structured deployment jobs.
- Agent image pull and Docker Compose/Engine update.
- Health checks.
- Release history and manual rollback.

Acceptance:
- A built image can be deployed to a test server.
- Release is marked healthy only after health check succeeds.
- Failure state includes stage and diagnostic logs.
- Rollback deploys a previous recorded digest.
- Persistent-volume and database migration risks are documented.

## Phase 5 — Production hardening

Build:
- Secret encryption and rotation plan.
- RBAC completion.
- Audit logs.
- Backup/restore procedure.
- TLS and secure installation defaults.
- Rate limiting and security headers.
- Agent credential revocation/rotation.
- Log and job retention.
- Upgrade strategy.

Acceptance:
- Backup can be restored in a clean test environment.
- Secret values are not returned by ordinary APIs or leaked into logs.
- Security checklist is reviewed.
- Upgrade and recovery steps are documented and tested.

## Phase 6 — Usability and performance

Build:
- Empty/error/loading states.
- Pagination and filtering.
- Log virtualization if required.
- SSE lifecycle cleanup.
- Accessibility and responsive refinements.
- Performance benchmarks.

Acceptance:
- Performance is measured under a documented test profile.
- UI remains usable with representative job/application counts.
- Idle resource use and build-time resource use are measured separately.

## Deferred features

Consider only after the MVP works:
- Approval gates.
- Automatic rollback.
- Blue/green or canary releases.
- Domain and TLS management.
- Notifications.
- Scheduled deployments.
- Historical metrics.
- OIDC/SSO.
- Organization-level ownership model.
- External Runner fleet management.

## Release readiness definition

The MVP is ready for a controlled test when the full golden path works:

```text
Git push
  → verified webhook
  → queued build
  → isolated test/build
  → image pushed
  → deployment job
  → Agent deploys digest
  → health check passes
  → release history updated
  → logs and rollback available
```

Do not call the product production-ready solely because the happy path works. Security, backup/restore, upgrade behavior, and failure recovery must also be tested.
