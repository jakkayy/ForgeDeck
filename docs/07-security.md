# 07 — Security Requirements

## 1. Security objectives

Protect source credentials, application secrets, production hosts, deployment permissions, and the integrity of releases. Assume the browser, repository contents, dependency install scripts, webhook requests, and network may be hostile.

## 2. Authentication and sessions

- Store password hashes using a modern password-hashing algorithm and appropriate parameters.
- Use secure, HttpOnly cookies for browser sessions.
- Enforce HTTPS in production.
- Apply CSRF protection consistent with the session design.
- Rate-limit login and other sensitive endpoints.
- Provide session invalidation/logout and safe credential reset flows.
- Do not log passwords, session tokens, or authorization headers.

## 3. Authorization

- Enforce permissions server-side for every project, application, server, job, and release.
- Avoid trusting IDs supplied by the browser without checking ownership/membership.
- Start with a documented role model (Owner, Admin, Developer, Viewer) and explicit permissions.
- Protect destructive actions with confirmation and audit records.
- Agents have machine identities and may only act on jobs and servers they are authorized for.

## 4. Git provider integration

- Prefer GitHub App installation tokens or other short-lived, least-privilege credentials where practical.
- Scope GitLab tokens to the required project and permissions.
- Encrypt stored provider credentials.
- Validate webhook signatures/secrets and deduplicate delivery IDs.
- Do not allow arbitrary repository URLs to bypass provider authorization.
- Rotate/revoke credentials when a connection is removed.

## 5. Runner threat model

A repository can contain malicious build scripts. A build can attempt to read files, steal credentials, attack internal services, exhaust resources, or escape its isolation boundary.

Required mitigations:
- Do not share the production Docker socket with untrusted builds.
- Use a dedicated build host or a strong isolation boundary.
- Set CPU, memory, disk, process, and time limits.
- Use temporary workspaces and clean them after execution.
- Keep credentials narrowly scoped and short-lived.
- Redact credentials from logs.
- Restrict network access where feasible.
- Avoid running builds as a privileged host user.

Container isolation alone should not be assumed to provide a perfect security boundary for hostile workloads.

## 6. Docker and Agent

- Never expose unauthenticated Docker Engine TCP.
- Keep Docker socket access local to the Agent where possible.
- Limit the Agent API to supported operations.
- Authenticate every Agent and support revocation/rotation.
- Use TLS for Agent-to-Control-Plane communication.
- Use short-lived enrollment tokens.
- Validate job payload schemas and protocol versions.
- Avoid arbitrary shell execution as a general deployment feature.
- Apply bounded timeouts and resource limits to operations.

## 7. Secrets

- Encrypt secrets at rest using authenticated encryption (AES-256-GCM since Phase 2, when Git provider tokens become the first stored secrets).
- Keep the encryption key outside PostgreSQL, for example in a protected environment/configuration source or external secret manager.
- Track key version to support rotation.
- Never return plaintext secret values in list APIs.
- Redact secrets from logs and errors.
- Avoid embedding secrets in job payloads unless required; prefer scoped secret references and controlled delivery.
- Ensure backups containing ciphertext are protected.

## 8. SSRF and network access

If users can configure URLs for health checks, Git endpoints, registries, or callbacks:
- Validate schemes and destination addresses.
- Block loopback, link-local, metadata-service, and unintended private destinations where relevant.
- Revalidate after DNS resolution and redirects.
- Avoid allowing user-controlled URLs to access internal Control Plane services.
- Set timeouts and response size limits.

The exact policy must account for legitimate private registries and internal health checks; implement explicit allowlists or scoped policies rather than a simplistic global block.

## 9. Web application security

- Validate and bound all input.
- Use parameterized SQL.
- Configure CORS narrowly.
- Use appropriate CSP and security headers.
- Protect file uploads and log rendering.
- Escape user-provided text in the UI.
- Rate-limit sensitive operations.
- Do not put tokens in query strings.

## 10. Audit and incident response

Audit:
- Login/security events.
- Project membership changes.
- Git connection changes.
- Agent enrollment/revocation.
- Secret creation/rotation/deletion metadata.
- Deployments and rollbacks.
- Container actions.
- Permission changes.

Audit metadata must not contain secret values. Document how to revoke provider credentials, disable an Agent, and restore from backup.

## 11. Production readiness checklist

- [ ] HTTPS enabled and certificates renewed safely.
- [ ] Secure session configuration and CSRF protection.
- [ ] RBAC enforced in service layer.
- [ ] Webhook signature validation and deduplication tested.
- [ ] Runner isolation reviewed.
- [ ] Docker socket not exposed publicly.
- [ ] Agent credentials protected and revocable.
- [ ] Secret encryption key stored outside database.
- [ ] Logs redacted and bounded.
- [ ] Database backup and restore tested.
- [ ] Dependency and image updates reviewed.
- [ ] Threat model reviewed before inviting untrusted projects.

## 12. Open decisions

- Exact encryption algorithm/key management implementation.
- Session storage and expiry policy.
- Whether external Runner hosts are supported in the first release.
- Network policy for build jobs.
- Secret rotation and incident response procedures.


## Audit log

Security-relevant changes are recorded in an append-only audit log that project admins read through `GET /projects/{id}/audit-logs` (see [03](03-database.md) and [04](04-api-spec.md)).

**What is recorded.** Every route that changes data is listed in `internal/audit/rules.go` as either *audited* or *exempt with a reason*, and a test walks the real router and fails if a route is in neither, so an endpoint cannot be added without deciding. Audited: projects and members, secrets (set, replace, delete), registries, Git connections, applications, builds, environments, deployments and rollbacks, job cancellation, server removal, enrollment tokens, container actions and log fetches, signing in. Recorded outside HTTP: an Agent enrolling (`server.enroll`, actor `agent`), a push that queues a build (`build.queue`, actor `webhook`), and an automatic deployment (`deployment.start`, actor `system`, written in the same transaction as the deployment). Exempt: heartbeats, claims, log events and completions from Agents and Runners (machine traffic; the work they do was audited when it was requested) and signing out.

**What is never recorded.** Secret values, passwords, tokens and credentials. Route rules copy only fields named in the rule (such as a role or a commit), the response is read only for the identifying fields `id`, `name`, `email` and a few like them (never an enrollment token or a session), and metadata values under keys that look like credentials are replaced with `[redacted]` as a last defense. Tests assert that known secret values appear nowhere in the log.

**Properties.** A failed request records nothing (an event says something happened). An event is written after the action; if writing fails it is logged at error level and the action stands. The actor is stored as text, so the record outlives the user. The log has no foreign key to the project, so deleting a project keeps the record of who deleted it; only the retention job removes rows. Reading it requires the admin role.

**Not covered.** There is no source IP in the record (ForgeDeck often sits behind a proxy, and a forwarded address is attacker-controlled unless the proxy is trusted); the request ID ties an event to the access log, which has the client address. Tampering by someone with database access is not detected: ship the log to a store they cannot write if that matters.

## Roles and permissions

Roles rank Viewer < Developer < Admin < Owner, and each role has everything below it. A caller who is not a member of a project gets **404** for anything in it (so membership cannot be probed); a member whose role is too low gets **403**; no session gets **401**.

| Capability | Viewer | Developer | Admin | Owner |
|---|---|---|---|---|
| See the project, members, servers, container list, Git connections, registries, secret **names**, applications, environments, deployments, releases, jobs and the output of builds and deployments | ✓ | ✓ | ✓ | ✓ |
| Start a build; deploy a build; roll back a release; cancel a job; restart, stop or start a container; fetch container logs and read them | | ✓ | ✓ | ✓ |
| Create, change or delete applications, environments, registries, Git connections and secrets; add or remove members; enroll or remove servers; read the audit log; change the project | | | ✓ | ✓ |
| Delete the project | | | | ✓ |

Notes on the choices:

- **Container output is for developers.** A fetched container log can hold sensitive data, so fetching it and reading it (the job's `/logs` and `/events`) need the developer role. Viewers can see that the job ran. This is stricter than the first version, which let viewers fetch logs.
- **Secret values are in no row of this table.** Nobody can read one back, including owners; admins can only replace or delete.
- A few routes belong to no project: listing one's own projects, servers and jobs (each returns only what the caller's memberships allow), creating a project (the creator becomes its owner), the caller's own user, sign-in and sign-out, the GitHub install redirect (verified by a sealed state and OAuth), the health check, and the embedded UI. Machine endpoints are authenticated by an Agent credential, a Runner credential, or a webhook signature.

**How this is enforced.** `internal/app/rbac_test.go` walks the real router. Every route must be classified there with its minimum role (or listed as not project-scoped, with a reason), or the test fails. For each project route it then calls the route as an anonymous caller (401), a non-member (404), a member one role below the minimum (403), and a member with exactly the minimum role (must not be refused), against real resources of every kind. Adding an endpoint without deciding who may call it fails the build. The review that introduced this test found three problems, now fixed: viewers could fetch container logs; the GitHub install route reported whether a GitHub App is configured before checking the caller's role, which told non-members about the installation; and fetched container logs were readable by viewers through the job endpoints.
