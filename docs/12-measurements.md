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

## Phase 1 — idle resource use

**Goal** ([README](README.md#resource-goals)): ForgeDeck fits on a 2 CPU / 4 GB host alongside PostgreSQL and a Runner.

### Setup

Same host as above. `make build` output (web UI embedded in the API binary), one project, one server whose Agent holds a long-poll claim open, no browser connected. Measured after two minutes idle.

### Results

| Process | Resident memory (RSS) | CPU |
|---|---|---|
| `forgedeck-api` (includes the dashboard) | 15.7 MB | ~0% |
| `forgedeck-agent` | 15.6 MB | ~0% |
| PostgreSQL 17 (Docker, default settings) | 84 MB | ~0% |

| Binary (static, `linux/amd64`) | Size |
|---|---|
| `forgedeck-api` | 12.9 MB |
| `forgedeck-agent` | 7.4 MB |

Dashboard bundle (gzip): initial JS 103 KB, CSS 9.3 KB; each page loads its own chunk.

### Notes

- Restarting the API was absorbed by the Agent: it logged two failed claims, backed off, reconnected, and the server returned to `online` without intervention.
- Memory under load (many Agents, large log fetches, concurrent users) is not measured yet; that belongs to the Phase 6 benchmark profile.

## Phase 6 — benchmark profile and results

**Goal** ([10](10-roadmap.md)): performance is measured under a documented test profile; the UI stays usable with representative job and application counts; idle and build-time resource use are measured separately.

### Test profile: "small production"

| | |
|---|---|
| Date | 2026-10-14 |
| Host | Intel Core i5-1145G7 (8 threads), 16 GB RAM, Linux; Control Plane, PostgreSQL and the load generator on the same host (no network latency) |
| Software | Go 1.26.0, PostgreSQL 17 (Docker, default settings), Chrome (headless, Playwright), production build with the dashboard embedded |
| Data | 10 projects, 50 servers, 100 applications, 200 environments, 8,000 deployments, 7,000 releases, 20,000 jobs (300,000 log lines on the newest 1,000), 50,000 audit events; database 122 MB |
| Load | 20 concurrent dashboard users (one session) reading as fast as the server answers, plus 50 simulated Agents, each with a heartbeat every 20 s and a 25 s job long-poll |

Repeat it: migrate an empty database, create `bench@forgedeck.local` with `forgedeck-api create-user`, load [scripts/bench/seed.sql](../scripts/bench/seed.sql) with `psql -v ON_ERROR_STOP=1 -f`, start the Control Plane, and run `go run ./scripts/bench -url http://127.0.0.1:8080`. The browser numbers come from Chrome driven by Playwright (largest-contentful-paint and Event Timing observers); the scripts are not in the repository.

### API latency, under load

Each endpoint was driven by 20 concurrent users for 8 seconds, with the 50 Agents running. Target: common reads p95 ≤ 200 ms ([08](08-frontend-ux.md#13-performance)).

| Endpoint | req/s | p50 ms | p95 ms | p99 ms |
|---|---|---|---|---|
| `GET /auth/me` | 25,200 | 0.7 | 1.5 | 2.1 |
| `GET /overview` | 985 | 20.0 | 23.9 | 27.1 |
| `GET /servers` (page of 25) | 4,780 | 4.1 | 6.2 | 7.5 |
| `GET /servers?q=…&status=online` | 4,340 | 4.5 | 6.6 | 7.9 |
| `GET /projects` | 9,280 | 2.0 | 3.2 | 4.0 |
| `GET /projects/{id}/applications` | 6,550 | 2.9 | 4.4 | 5.4 |
| `GET /projects/{id}/applications?q=…` | 5,790 | 3.2 | 5.5 | 8.0 |
| `GET /jobs` (page of 25) | 1,880 | 9.8 | 17.5 | 24.8 |
| `GET /jobs?status=failed` | 1,610 | 11.9 | 17.4 | 23.3 |
| `GET /jobs?application_id=…&type=build` | 1,240 | 15.3 | 24.1 | 30.4 |
| `GET /jobs/{id}/logs` (100 lines) | 3,500 | 5.6 | 7.9 | 9.7 |
| `GET /environments/{id}/deployments` | 960 | 20.5 | 24.3 | 27.4 |
| `GET /environments/{id}/releases` | 2,630 | 7.4 | 10.0 | 12.5 |
| `GET /projects/{id}/audit-logs` | 3,690 | 5.3 | 7.6 | 9.0 |
| `GET /servers/{id}` | 8,290 | 2.3 | 3.4 | 4.1 |
| **Mixed**: every user cycles through all 15 | 2,330 | 5.7 | 38.4 | 47.6 |

No request failed. The 50 Agents sent 450 heartbeats and 300 long-poll claims without error.

**What the benchmark found and fixed.** The first run met the target on average but not everywhere: `GET /jobs` took 209 ms (p50) and `GET /overview` 153 ms, and the mixed p95 was 130 ms. Both read every row of a large table (every job, every deployment) and sorted it to return 25. They now ask each project (jobs) or each environment (overview) for only its newest few rows from an index and merge those, so the cost follows the page size rather than the table size. After the change: `/jobs` 9.8 ms, `/overview` 20 ms, mixed p95 38 ms. The same run showed the dashboard being sent uncompressed (250 KB of JavaScript instead of 78 KB) when the binary is run without a proxy; it is now compressed by the Control Plane itself (the production Compose stack also compresses at Caddy).

### Dashboard (Chrome)

Cold cache, signed in, the profile's data. Target: LCP ≤ 2.5 s, INP ≤ 200 ms ([08](08-frontend-ux.md#13-performance)).

LCP, median of 5 loads:

| Page | Desktop, no throttling | Mobile profile |
|---|---|---|
| Overview | 152 ms | 2.03 s |
| Servers | 188 ms | **2.71 s** |
| Projects | 172 ms | 2.47 s |
| Jobs | 168 ms | 2.41 s |
| Project | 232 ms | **3.12 s** |
| Application | 180 ms | **2.73 s** |
| Environment | 208 ms | **2.53 s** |
| Job log (300 lines) | 188 ms | **2.68 s** |

The mobile profile is deliberately harsh: 390 px viewport, **4× CPU slowdown, 1.6 Mbit/s down, 150 ms round trip**. Under it, **5 of 8 pages miss the 2.5 s target** (bold), by 0.03–0.6 s; on desktop all are well under 0.25 s. This is the first load with nothing cached; hashed assets are cached for a year, so later page loads inside the app are client-side navigation. Not done: preloading route chunks, or a smaller initial bundle (107 KB gzip of JavaScript, 20 KB of CSS today), which are the levers if this profile matters.

Interaction latency (worst Event Timing duration; interactions faster than 16 ms are not reported by the browser, shown as under 16):

| Interaction | Desktop | Mobile (4× CPU) |
|---|---|---|
| Type a search term (7 keystrokes) | 16 ms | 32 ms |
| Open the status filter and pick one | 56 ms | 136 ms |
| Load more servers | < 16 ms | < 16 ms |
| Switch project tab | 32 ms | 96 ms |
| Scroll a log and *Copy all* | 16 ms | 16 ms |

All under the 200 ms INP target. A 200,000-line log keeps about 60 rows in the DOM (see [08](08-frontend-ux.md#9-logs)).

### Resource use

Measured separately, as the roadmap asks.

| Situation | Control Plane (`forgedeck-api`) | PostgreSQL |
|---|---|---|
| Idle, no Agents | 14.7 MB RSS, ~0% CPU | |
| 50 Agents connected, no users | 0.5% of one core; 34 MB RSS (measured after the load run, so it still holds the memory the load made it grow) | ~150 MB, 0.02% CPU |
| Under the load above (about 165 s) | peak 65 MB RSS, about 2.3 cores on average | |

Build-time use is the Docker build's, not the Control Plane's: a cold `docker build --no-cache` of ForgeDeck's own image (a web build with Bun, then a Go build) took **38 s**, used **89 CPU-seconds (2.4 cores on average)**, and raised the host's memory use by a peak of **401 MB**. This is a whole-host measurement over the build (the work happens in the Docker daemon, outside any ForgeDeck process), so other activity on the host is included; the Runner itself only clones and drives `docker`. A larger application will use more. Builds run on the Runner host, so this load does not land on the Control Plane's host.

### Not covered

- Control Plane, database and clients on different hosts, or a real network between them.
- More than 50 Agents or 20 simultaneous users, and data beyond the profile (the new queries scale with the page size and the number of projects/environments, not with the number of rows, but this was not measured at 10× the data).
- Live job logs and state changes over SSE under load (container action latency was measured in Phase 1).
- A real Runner under concurrent builds, and the memory of builds that run tests.
- A screen reader pass for accessibility ([08](08-frontend-ux.md#14a-accessibility)).
