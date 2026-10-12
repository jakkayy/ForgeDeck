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
