# 08 — Frontend UX and Design System

## 1. Goal

The UI should feel like a modern, friendly developer tool: clean, minimal, fast, and clear. It should not look like a wall of monitoring charts or an overly complex enterprise console.

## 2. Stack

- Vite (build only)
- React + TypeScript
- TanStack Router
- Tailwind CSS
- HeroUI
- TanStack Query
- SSE for live job/deployment updates

The app is a client-rendered SPA. `vite build` produces static files that the Control Plane embeds with Go `embed` and serves from the same origin as the API. Unknown non-API paths fall back to `index.html` so client-side routes work on reload.

Why not Next.js: every page is behind login (no SEO need), business logic already lives in the Go API, and a Node.js server would add a second runtime process, a second origin for cookies/SSE, and more memory on small installations.

Choose compatible versions at implementation time and document the selected versions in package manifests. Avoid adding libraries that duplicate existing functionality.

## 3. Navigation model

Suggested primary navigation:
- Overview
- Projects
- Applications
- Servers
- Deployments / Jobs
- Settings

Within an application:
- Overview
- Deployments
- Logs
- Settings

Keep global navigation stable and avoid deeply nested menus.

### Builds (Phase 3)

- The project page has **Applications · Git · Registries · Members** tabs. Registries holds the places images are pushed; an application chooses one and an image path.
- An application page shows its image destination, a **Build <branch>** button (developer and up; disabled until an image is set), and recent builds. A row shows the short commit, its message, status, the pushed digest or the failure code, and the duration. The list refreshes every 5 seconds only while a build is queued or running.
- The application page also lists **environments**: for each, its server and port mapping, what it runs now (short commit and image digest, since when), the last deployment's status, and **Deploy** (developer and up), **Edit** and **Delete** (admin). Deploy ships the newest successful build and asks for confirmation first, saying that the service restarts briefly and that an unhealthy version is rolled back. The environment form keeps the common fields (name, server, ports, auto-deploy) up front and tucks bind address, health check, environment variables, volumes, restart policy and memory under *Advanced*. Auto-deploy is off by default.
- A new project shows a **getting-started guide** above its tabs (admins and owners only, since only they can do the steps): connect Git, add a registry, add a server and install its Agent, create an application, add an environment, build and deploy. Each step is checked against what exists, not remembered: one that is done is struck out the moment it is, the first one not done is highlighted as the next, and each has a button that goes where it is done. The guide disappears by itself once a release is running (it looks at up to five applications to decide). It does not create the first user: that is `forgedeck-api create-user`, on purpose. A web form that makes the first account would let whoever reaches a fresh server first take it over; if it is ever added it needs a one-time token printed to the server's log.
- The project page has a **Secrets** tab: names with version and age, **Add secret**, **Replace**, and **Delete** (admin). The value field is a password input, the page says that values can never be read back and that a change reaches each service at its next deployment, and deleting is refused while an environment lists the secret. The environment form lists the project's secrets as checkboxes (under *Advanced*); a secret that no longer exists but is still listed stays visible so it can be unticked.
- Admins and owners also see an **Audit log** tab: each event reads as a sentence ("owner@example.com replaced the secret DATABASE_PASSWORD") with its action code and age (the exact time is in the tooltip), filterable by action and loaded 50 at a time. Other roles do not see the tab and the API refuses them.
- An **environment page** (`/environments/{id}`) shows what runs now and the environment's deployments, newest first. Each deployment draws the pipeline `Queued → Pulling image → Starting → Health check → Healthy`: finished stages are checked, the current one pulses, a failure marks the stage it failed at in red and leaves later stages grey, and a cancelled deployment claims nothing. The list refreshes every 3 seconds only while a deployment is queued or running. Each row links to the job log.
- The server page has **Rotate credential** next to **Remove server** (admin): after a confirmation the Agent replaces its credential at its next heartbeat and saves it itself, with nothing to copy; the old one works for 15 minutes.
- Below the deployments, **Release history** lists every release (commit, image, age, a *current* chip). Developers and above can **Roll back** to any other release. The confirmation says that the exact image and its configuration are redeployed without rebuilding, that the service restarts briefly, and in bold that **data is not rolled back**: volumes and database changes made by newer versions stay, and an old version may not be able to use migrated data.
- A **failed deployment** explains itself in place: a title naming what failed and at which stage, the server's own message (which says what state the environment is in: the previous release restored, restoring failed too, or no previous release), a short list of likely causes and fixes for that error code, the code itself, and a link to the log. Codes without specific advice get a generic message and the log link. Deploy jobs read as `Deploy <commit> · <image>` in the jobs list and link back to their environment.
- A build's job page shows the commit, the exact `image@sha256:…` reference with a copy button once pushed, and the streamed log.

## 4. Overview page

Prioritize:
- Online/offline servers.
- Running/healthy applications.
- Recent deployments.
- Recent failures requiring action.
- Recent activity.

Avoid large decorative charts. Only add a graph if it answers a real operational question.

The page at `/` shows four counts (servers online, servers down, environments whose last deployment succeeded, failed deployments), a *Needs attention* list (servers that stopped reporting and environments whose newest deployment failed, each linking to the environment), the eight newest deployments, and the six newest jobs. It covers every project the user belongs to and refreshes every 20 seconds. A user with no servers or environments sees a pointer to Projects instead of zeros. "Running" means the last deployment passed its health check; the page does not claim to know that a service is up at this moment.

## 5. Application list

Each row should make it easy to scan:
- Application name.
- Repository and branch.
- Environment.
- Current health/release.
- Last deployment.
- Clear actions such as View, Deploy, Logs, Settings.

Use pagination or filtering for large lists.

Every list that can grow loads 25 rows at a time with **Load more**: projects, servers, applications, builds, deployments, releases, jobs, and the audit log. Searchable lists (projects, servers, applications) have a search field that asks the API after 300 ms without typing, so the filter applies to everything, not only the rows already loaded; servers also filter by status, and jobs by status and type. The previous rows stay on screen while a new filter loads, and an empty result says that nothing *matches* rather than that nothing exists. Pickers inside dialogs (a project or a server to choose) show the first 100 entries.

## 6. Application detail

Show:
- Current status and environment.
- Repository/branch.
- Current release and commit.
- Last deployment result.
- Primary actions.
- Recent deployment history.

Use confirmation for destructive or risky actions. Do not make Restart, Delete, and Deploy visually indistinguishable.

## 7. Deployment detail

Show the pipeline stages:
`Queued → Building → Pushing Image → Deploying → Health Check → Healthy`

For failure, identify the stage, show a concise reason, and link to logs. Distinguish an in-progress state from a failed state and from a healthy release.

## 8. Server detail

Show:
- Connection state and last heartbeat.
- Agent version.
- Docker availability.
- Resource summary when available.
- Container list and status.
- Recent Agent events.

Avoid querying expensive Docker stats continuously. Make metric freshness visible where useful.

## 9. Logs

- Use a readable monospace font.
- Preserve timestamps and stream/source information.
- Support auto-follow with a pause option.
- Virtualize long output.
- Allow copying a selected range.
- Show truncation/retention notices.
- Escape log content; do not render it as HTML.
- Redact secrets before they reach the browser.

## 10. Design system

Use HeroUI components consistently for:
- Buttons
- Inputs
- Selects
- Tables
- Modals
- Drawers
- Tabs
- Chips/status labels
- Tooltips
- Toasts
- Progress indicators
- Skeleton loading

Define design tokens for spacing, typography, border radius, and semantic status colors. Keep color usage restrained and preserve accessible contrast.

## 11. UI states

Every data-driven page should support:
- Loading
- Empty
- Success
- Error
- Stale/disconnected
- Permission denied
- Partial data where applicable

Empty states should explain the next action, e.g. “No servers yet — register your first Docker server.”

How the dashboard implements them:

- **One component for lists and sections.** `QueryBoundary` (and `waitingFor` / `StaleNotice` for pages that return early) renders loading, failed, empty and stale in the same way everywhere, so a page cannot forget one.
- **Loading** is a spinner labelled for screen readers ("Loading servers"). **Failed** with nothing to show is an error with the server's message and *Try again*.
- **Stale:** when a refresh fails, or the browser is offline, the last data stays on screen under a notice saying how old it is ("Could not refresh servers. Showing what was loaded 2 minutes ago."). A network blip never replaces a list with an error screen. Pages refresh by polling (servers, containers, deployments, releases, jobs), so this is the state a user meets most.
- **Permission denied:** a project you do not belong to is shown as *not found* (the API does the same, so IDs cannot be probed); actions above your role are hidden or disabled rather than failing.
- **A URL that matches no page** shows *Page not found*, and a page that fails while rendering shows the error with *Try again*, instead of a blank screen.

## 12. Interaction principles

- One obvious primary action per page.
- Use progressive disclosure for advanced settings.
- Provide immediate feedback after an action.
- Do not report success before the server confirms success.
- Use optimistic updates only for safe operations.
- Confirm destructive actions and explain consequences.
- Keep keyboard navigation and accessible labels in mind.

## 13. Performance

- Split code per route so a page only loads what it uses.
- Enforce a bundle size budget in CI (`bun run size`: initial JS ≤ 130 KB, CSS ≤ 20 KB, each route chunk ≤ 60 KB, all gzip). The CSS budget was raised from 15 KB in Phase 2: all component CSS ships in one global file and grows about 1–2 KB per HeroUI component used. Splitting component CSS per route is the next step if it keeps growing (Phase 6) and review it when adding dependencies. HeroUI and its animation dependency are the largest expected contributors; import components individually and measure.
- Serve hashed assets with long-lived cache headers and `index.html` with no-cache.
- Fetch only the data needed for the current page.
- Use pagination and server-side filtering.
- Avoid aggressive global polling.
- Subscribe to SSE only while a relevant detail/log page is open.
- Cancel requests when no longer needed.
- Avoid large animation packages and unnecessary re-renders.
- Use virtualization for large logs/tables where necessary.
- Measure production builds rather than relying on assumptions.

Suggested initial targets to validate:
- LCP ≤ 2.5 seconds under a defined test profile.
- INP ≤ 200 ms under a defined test profile.
- Common read API p95 ≤ 200 ms in a controlled test environment.
- Live state changes visible within 1–3 seconds when the connection is healthy.

These are targets, not guarantees.

## 14. Responsive behavior

Prioritize desktop for server administration, while keeping core flows usable on tablet and mobile. On small screens, simplify tables and move secondary actions into a menu instead of forcing horizontal overflow.

## 15. Open decisions

- Light mode, dark mode, or both.
- Exact typography and color palette.
- Whether the Overview is global or project-scoped by default.
- Which table library is needed beyond HeroUI's components.
