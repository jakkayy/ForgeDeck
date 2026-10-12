# 01 — Product Specification

## 1. Summary

ForgeDeck is a self-hosted DevOps control plane for building and deploying Docker-based applications. It consolidates repository integration, CI execution, image publishing, Docker server management, deployment history, logs, and rollback into one user interface.

**Product promise:** deploy Docker applications without manually managing every step over SSH.

## 2. Problem

A typical small team may combine GitHub/GitLab, CI pipelines, container registries, SSH, Docker Compose, server monitoring, and log inspection. This increases context switching and makes deployment workflows harder to understand.

ForgeDeck aims to provide one understandable workflow and a clear view of what is running, what changed, and why a deployment failed.

## 3. Target users

- **Individual developers:** run personal projects on self-managed servers.
- **Small engineering teams:** deploy multiple applications across staging and production.
- **Server administrators:** register servers and see Docker workload status.
- **Organizations:** self-host an independent ForgeDeck installation with project membership and role-based permissions.

## 4. Product principles

1. **Easy to use:** sensible defaults and a short happy path.
2. **Lightweight:** avoid unnecessary services and background work.
3. **Docker-native:** Docker Engine and Compose first.
4. **Safe by default:** separate build execution from production.
5. **Transparent:** show job stages, logs, health, and release history.
6. **Self-hosted:** data and control plane live in the operator's environment.

## 5. MVP scope

### Included

- Local account login/logout and protected sessions.
- Projects and basic project membership.
- GitHub and GitLab repository connections.
- Webhook validation and event deduplication.
- Server registration and Agent enrollment/revocation.
- Server heartbeat and basic resource status.
- List Docker containers and view logs; start/stop/restart with authorization.
- Job queue, attempts, leases, cancellation where safe, and retry rules.
- Separate Runner for checkout, tests, image build, and registry push.
- Application and environment configuration.
- Deployment via Docker Engine/Compose.
- Health checks, deployment history, image digest, and manual rollback.
- SSE for live job/deployment logs.
- Audit records for important actions.
- Responsive dashboard built with Next.js, TypeScript, Tailwind CSS, and HeroUI.

### Explicitly out of scope for MVP

- Kubernetes or cluster management.
- Hosting Git repositories.
- Cloud resource provisioning.
- Terraform orchestration platform.
- Arbitrary browser-based shell access.
- General-purpose CI YAML compatibility engine.
- Multi-region scheduling, service mesh, and advanced canary orchestration.
- Built-in Prometheus/Grafana/Elasticsearch stack.
- A central multi-tenant SaaS control plane.

## 6. Core entities

- **User:** a person who signs in.
- **Project:** a logical grouping of applications and collaborators.
- **Git connection:** authorized connection to GitHub or GitLab.
- **Application:** a deployable software component associated with a repository.
- **Environment:** a target context such as development, staging, or production.
- **Server:** a machine registered with ForgeDeck.
- **Agent:** the authenticated process that operates Docker on a server.
- **Job:** a unit of work such as build, deploy, or container action.
- **Release:** a deployment record referencing a specific image digest and commit.

## 7. Primary user journey

1. Sign in.
2. Create a project.
3. Connect a Git provider and select a repository.
4. Create an application and choose build settings.
5. Register a Docker server and install its Agent.
6. Configure an environment and registry credentials.
7. Push code or manually trigger a build.
8. Watch build and deployment stages.
9. Confirm the health check and inspect the release.
10. View logs or roll back to a previous known-good release.

## 8. Success criteria

The MVP is useful when a user can complete the primary journey end-to-end, see actionable failure information, and redeploy a previous release without manually rebuilding the same image.

Suggested initial usability goals:
- A new user can identify the next action on each primary page without reading a long manual.
- Deployment state and current release are visible at a glance.
- Failed deployments show the failed stage and relevant logs.
- Destructive operations require clear confirmation.
- The dashboard stays responsive with paginated lists and scoped live updates.

These are goals to validate, not guaranteed performance results.

## 9. Product metrics to measure

- Time from push to healthy release.
- Build success/failure rate.
- Deployment success/failure rate.
- Time to identify the failing stage.
- Agent connectivity and job lease expiry counts.
- Dashboard interaction latency under a representative workload.
- Idle and active CPU/RAM usage on the self-hosted control plane.

## 10. Open product decisions

- Whether the first release supports only one local admin or multiple users from day one.
- Which registry is the first documented happy path (GHCR is a reasonable candidate).
- Whether Compose configuration is generated by ForgeDeck or supplied by the user.
- Which Linux distributions are officially supported for Agent installation.
- Whether automatic rollback is enabled by default or remains a manual action in MVP.
