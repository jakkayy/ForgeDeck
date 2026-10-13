# 17 — Upgrade Guide

How to move a running ForgeDeck to a newer version without losing data or leaving machines behind.

**The short version:** take a backup and check that it restores, upgrade the Control Plane (migrations run by themselves), then upgrade each Agent and the Runner. Applications that are already deployed keep running throughout.

## Before you start

1. **Back up, and verify the backup.** An upgrade applies database migrations that cannot be undone by starting the old version again (see [Going back](#going-back)); the backup is your way back.

   ```sh
   FORGEDECK_PG_CONTAINER=forgedeck-prod-postgres-1 deploy/backup/backup.sh -d /var/backups/forgedeck
   FORGEDECK_PG_CONTAINER=forgedeck-prod-postgres-1 deploy/backup/restore.sh --verify /var/backups/forgedeck/<the new file>
   ```

   ([16](16-backup-restore.md) explains both, and why `FORGEDECK_SECRET_KEYS` must be kept separately.)
2. **Read [what changed](#what-changed-and-what-to-do) for the versions you are skipping.** Anything that needs action is listed there.
3. **Know what you run now:** `docker compose exec api /forgedeck-api version`.
4. **Check disk space** for the new image and a database that may grow slightly during a migration.
5. **Pick a quiet moment.** The Control Plane is unreachable for a few seconds to a minute while it restarts and migrates.

## The order

| Step | What | Why in this order |
|---|---|---|
| 1 | **Control Plane** | It owns the database and the migrations. |
| 2 | **Agents** (each server) | They speak the Agent protocol to the Control Plane. |
| 3 | **Runner** (build host) | Same protocol. |

**Mixed versions are fine while the protocol version is unchanged.** Every release so far speaks Agent protocol **1**. A Control Plane that is newer than its Agents and Runners keeps working with them, so you can upgrade the Agents at your own pace, one server at a time. Whenever a release changes the protocol incompatibly, its notes below say so and give the order to follow; until they do, there is nothing to coordinate. (A mismatch is refused with `unsupported_protocol`, never silently misread.)

## 1. The Control Plane

With the supported Compose setup ([deploy/compose](../deploy/compose/README.md)):

```sh
cd ForgeDeck
git fetch --tags
git checkout <the version you are upgrading to>
cd deploy/compose
FORGEDECK_VERSION=$(git describe --tags --always) docker compose up -d --build
```

This builds the new image, restarts the Control Plane on it, and leaves PostgreSQL and the proxy running. On start the Control Plane **applies pending migrations before it serves any request**. Each migration runs in its own transaction: if one fails, it is rolled back, the Control Plane exits with the reason, and it does not serve. Fix the cause (or go back) rather than forcing it.

Watch it come up:

```sh
docker compose logs -f api                              # "forgedeck starting" with the version, then migration lines
docker compose exec api /forgedeck-api version
docker compose exec api /forgedeck-api migrate status   # every migration "applied"
```

### What happens to work in progress

The restart is short, and ForgeDeck is built to ride it out:

- **Deployed applications are not touched.** They run on the servers; ForgeDeck is not in the request path.
- **Agents and Runners** retry with backoff and reconnect by themselves; long-polls simply start again.
- **A running build or deployment** continues on its machine. Its lease (60 seconds) cannot be renewed while the Control Plane is down. If the Control Plane is back within the lease, nothing is lost. If not, the job is recovered by the usual rule: builds and deployments are retried (both are safe to repeat), a container restart is not retried automatically. So keep the restart under a minute, and avoid upgrading in the middle of a deployment you care about.
- **Browsers** show errors until it is back; a reload is enough. The dashboard is part of the binary, so it is upgraded with it; there is nothing separate to deploy, and a new `index.html` is picked up at once (it is served with `no-cache`).

## 2. Each Agent

On every server, with the new `forgedeck-agent` binary:

```sh
sudo ./install.sh --binary ./forgedeck-agent
```

It replaces the binary and the unit, restarts the service, and keeps the Agent's identity ([deploy/agent](../deploy/agent/README.md)). The server shows as online again within a minute. A deployment running during the restart is retried by the Control Plane after its lease expires.

You can leave Agents on the old version for a while. Features that need a newer Agent say so: for example, a server whose Agent predates deployments fails a deploy with `unsupported_job_type`, and the dashboard explains that the Agent must be upgraded.

## 3. The Runner

Replace the binary and restart the service (`sudo systemctl restart forgedeck-runner`). A build in progress is abandoned and retried by the Control Plane.

## After the upgrade: check

- `forgedeck-api version` shows the version you meant.
- `migrate status` lists everything as applied.
- You can sign in, and each server is **online**.
- **Re-run the backup verification** on a fresh backup: it proves the new version's schema dumps and restores.
- Trigger a build and a deployment to a staging environment.

## Going back

**The database only moves forward.** Migrations add tables and columns, and the old version does not know about them: starting an older binary on a migrated database is **not supported**, even though it may appear to work, and it can corrupt data it does not understand.

To go back after a migration has been applied:

1. Stop the Control Plane (`docker compose stop api`).
2. Restore the backup you took before upgrading, into the database ([16](16-backup-restore.md#restoring)), with the same `FORGEDECK_SECRET_KEYS`.
3. Check out the previous version and `docker compose up -d --build`.

You lose whatever happened after the backup (deployments, job history, audit events). If the old version never applied a migration (the new one failed on the first), you can simply start the old version again: `migrate status` tells you.

## Upgrading PostgreSQL itself

Moving to a new PostgreSQL major version (for example 17 → 18) is a dump and restore, not an in-place change of the data directory:

1. Back up and verify, as above. Stop the Control Plane.
2. Change the image tag in `docker-compose.yml` and start a **new, empty** PostgreSQL volume (a different volume name, so the old data is kept untouched until you are sure).
3. Restore the backup into it with `restore.sh --yes`.
4. Start the Control Plane, check, and delete the old volume only when you are satisfied.

Minor versions (17.x) are a plain image update.

## Upgrading the machines themselves

- **Docker on an Agent host:** restarting the Docker daemon stops containers unless `live-restore` is on. Services deployed by ForgeDeck use the restart policy `unless-stopped` by default, so they come back; the Agent restarts through systemd. Do hosts one at a time.
- **The operating system:** the same, plus a reboot: the Agent and the deployed services start again by themselves.
- **The Control Plane host:** Compose restarts the stack after a reboot (`restart: unless-stopped`). Certificates are kept in the `caddy-data` volume.

## What changed, and what to do

Only changes that need you to act, or that you could notice, are here.

### Development builds → the first hardened release (Phase 5)

If you ran ForgeDeck from the development branch before this release:

| Change | What to do |
|---|---|
| **Retention cleanup is on.** About a minute after the Control Plane starts, and every 6 hours after, it deletes job logs older than 30 days, finished jobs older than 180 days, webhook deliveries older than 30 days, and audit events older than 365 days. Releases and deployments are never deleted. | If you need to keep more, set `FORGEDECK_RETENTION_*_DAYS` (0 keeps forever) **before** you upgrade ([03](03-database.md#6-retention)). |
| **Behind a reverse proxy, rate limits need the proxy's address.** Without it every client looks like the proxy and shares one limit. | Set `FORGEDECK_TRUSTED_PROXIES` to the proxy's address or range. The supported Compose setup already does. |
| **Container logs are for developers and above.** Fetching them, and reading a fetched log, no longer works for viewers. | Give people who need them the developer role. |
| **Secrets have a proper place.** Environment variables in an environment are visible to every project member. | Move anything sensitive into project **Secrets** and list it on the environment; nothing is moved for you. |
| **Stricter browser security.** A Content-Security-Policy forbids inline scripts and other origins. | If a proxy or browser extension injects scripts into the dashboard, it will be blocked. |
| **Agent credentials rotate by themselves** every 90 days, or when an admin asks. | Nothing. Agents must be able to write their state directory (the installer does this). |
| **Most changes by a signed-in user are rate limited**, and sensitive ones (adding members, connecting Git) more tightly. | Nothing, unless a script drives the API in bursts of more than about 60 changes. |
| **Audit log.** Recorded from the moment of the upgrade; earlier actions are not in it. | Admins find it in the project's *Audit log* tab. |
| The Control Plane image now runs as a non-root user with a read-only filesystem. | Nothing for the supported Compose setup. If you ran your own container, make sure it can reach PostgreSQL and needs no writable path other than `/tmp`. |

There are no protocol changes: Agents and Runners from the development builds keep working.

### Template for future releases

Each release adds a section here: the version, whether the Agent protocol changed (and the order to follow if it did), any migration that is slow or needs care, and every change that needs action.

## If an upgrade fails

| Symptom | What it means | What to do |
|---|---|---|
| The Control Plane exits at start with a migration error | The migration was rolled back; the database is as before the attempt | Read the error in `docker compose logs api`. Fix the cause, or go back to the previous version (it never applied anything, so it can start) |
| It starts, but sign-in or the dashboard misbehave | Possibly a stale browser cache | Hard-reload; `index.html` is never cached, but an extension may |
| Agents stay offline | A protocol change not announced here, or the proxy | `docker compose logs api` shows `unsupported_protocol` if it is the first; check the proxy otherwise |
| Stored Git tokens or registry passwords stop working | `FORGEDECK_SECRET_KEYS` is not the key that was in use | Put the original key back (keep old keys in the list); [16](16-backup-restore.md) |

The one upgrade property that is tested in the repository: data written under an older schema survives the migrations that a newer version applies, and the new tables and columns start with safe values (`go test ./internal/database/`).
