# AI Coding Guidelines

These instructions apply to AI coding agents working in the ForgeDeck repository.

## Before changing code

1. Read `README.md` and the relevant files under `docs/`.
2. Check the current code before assuming an API, table, package, or feature exists.
3. If implementation conflicts with documentation, explain the discrepancy and propose a small, explicit correction.
4. Keep scope aligned with the current roadmap milestone.

## Core architecture rules

- Use Go for the backend, Runner, and Agent.
- Keep the Control Plane a modular monolith until there is a demonstrated reason to split it.
- Use PostgreSQL as the initial source of truth and job queue; do not add Redis, Kafka, or Temporal without an approved requirement.
- Use Docker Engine / Docker Compose for the MVP. Do not introduce Kubernetes into the MVP.
- Keep CI build execution separate from production deployment.
- Production Agents initiate outbound HTTPS connections to the Control Plane; do not require an inbound public port on production servers.
- Never expose an unauthenticated Docker Engine TCP endpoint.
- Do not mount the host Docker socket into untrusted build jobs.

## Implementation practices

- Prefer small, cohesive changes and clear package boundaries.
- Validate input at API boundaries.
- Use context cancellation and timeouts for network and Docker operations.
- Use structured logs; never log credentials or secret values.
- Use database migrations for schema changes.
- Add tests for state transitions, authorization, retries, and error cases.
- Keep API request/response contracts and documentation synchronized.
- Use pagination for potentially large collections.
- Stream live logs with SSE only when needed; avoid aggressive polling.
- Make deployment operations idempotent where practical.
- Pin deployments to image digests when available, not mutable tags alone.

## Security rules

- Treat repository code and build scripts as untrusted input.
- Verify Git provider webhook signatures and deduplicate event deliveries.
- Use least-privilege provider credentials.
- Encrypt stored secrets using a key kept outside the database.
- Redact secrets from logs.
- Enforce authorization on every project-scoped resource.
- Protect against SSRF when fetching user-controlled URLs.
- Use resource limits and timeouts for builds.
- Never claim a feature is secure merely because it has authentication; document the threat and mitigation.

## Definition of done

A change is done when:
- It meets the relevant acceptance criteria.
- Tests or checks have been run, and their results are reported accurately.
- Errors and edge cases are handled.
- Documentation is updated if behavior or contracts changed.
- No secrets or generated artifacts were committed accidentally.
