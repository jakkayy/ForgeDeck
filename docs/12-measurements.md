# 12 — Measurements

Results of measurements the roadmap asks for. Each entry states the setup so it can be repeated; numbers from one machine are a baseline, not a guarantee.

## Phase 1 — container action latency

**Target** ([08](08-frontend-ux.md#13-performance), [10](10-roadmap.md)): a container action is visible within 1–3 seconds on a healthy connection.

### Setup

| | |
|---|---|
| Date | 2026-10-13 |
| Host | Intel Core i5-1145G7, 16 GB RAM, Linux |
| Software | Go 1.26.0, PostgreSQL 17.11, Docker 29.8.2 |
| Topology | Control Plane, PostgreSQL, Agent, and Docker on the same host (no network latency) |
| Workload | `restart` of an idle `nginx:alpine` container, 10 runs, 0.5 s apart |

### Method

A script drives the public API as a signed-in user: it requests the action, polls `GET /jobs/{id}` every 20 ms until the job finishes, then polls `GET /servers/{id}/containers` until a snapshot observed after the request shows the container running again.

- **Pickup**: job `started_at − created_at`, the time from queueing until the Agent's long-poll claim returns it.
- **Completion**: from sending the request until the job is `succeeded` (includes Docker's restart).
- **Visible**: from sending the request until the dashboard's container list reflects the new state.

The dashboard was also measured end to end in Chrome with Playwright: from confirming the dialog until the success toast appears.

### Results

| Stage | Median | p90 | Max |
|---|---|---|---|
| Pickup (queue → Agent) | 9 ms | 9 ms | 10 ms |
| Completion (request → job succeeded) | 211 ms | 234 ms | 236 ms |
| Visible (request → new state in API) | 252 ms | 259 ms | 259 ms |
| Dashboard (confirm → success toast), 3 actions | 0.8–1.1 s | | |

### Notes

- Pickup in milliseconds confirms the long-poll is woken by PostgreSQL `NOTIFY` rather than the 5-second recheck timer.
- The dashboard figure is higher than the API figure because the UI polls the job every 500 ms; it stays inside the 1–3 s target. SSE in Phase 2 removes that polling delay.
- Restart time depends on the container: one that ignores SIGTERM waits for Docker's 10-second stop timeout. That is Docker behavior, not queue latency.
- Not covered here: Agent and Control Plane on different hosts, slow links, or many concurrent Agents. Re-measure under the Phase 6 test profile.
