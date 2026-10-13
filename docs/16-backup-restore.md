# 16 — Backup and Restore

ForgeDeck keeps all of its state in one PostgreSQL database. Backing it up, plus **one secret that is not in it**, is enough to rebuild a Control Plane.

## What to back up

| What | Where it is | Back it up? |
|---|---|---|
| The database: users, projects, servers, applications, environments, releases, deployments, jobs, audit log, and **encrypted** Git tokens, registry passwords and project secrets | PostgreSQL | **Yes**, with `deploy/backup/backup.sh` |
| **`FORGEDECK_SECRET_KEYS`** | an environment variable or `.env` on the Control Plane host; deliberately *not* in the database | **Yes, separately.** See below. |
| Your `.env` / Compose configuration (URLs, GitHub App settings, limits) | the Control Plane host | Yes, with the key (it holds secrets: store it as carefully) |
| Container images | your registry | Not by ForgeDeck. Rollback needs the images of past releases to still be there. |
| Data of your applications (volumes, their databases) | the servers | **No.** ForgeDeck does not back up what it deploys. See [15 §3](15-deployments.md#3-risks-volumes-and-database-migrations-418). |
| An Agent's identity (`/var/lib/forgedeck-agent`) | each server | Not needed: if lost, enroll the Agent again. |
| A Runner's credential | the build host | Not needed: create a Runner again. |

### The key is the one thing that cannot be recovered

Git tokens, registry passwords and project secrets are stored encrypted with `FORGEDECK_SECRET_KEYS`. The backup contains the encrypted values and **not the key**, so a stolen backup alone reveals none of them, and a backup without the key cannot be decrypted by anyone, including you.

If you restore a database and do not have the key that was in use when the backup was taken:

- everything else works (users, projects, history, releases, deployments);
- every stored Git connection, registry and project secret is unreadable: builds fail with a credentials error until you **enter them again**; webhooks from GitLab connections stop verifying until the connection is re-created.

So: **keep the key in a password manager or secret store, outside the Control Plane host and outside the backup location.** When you rotate keys (`id:key,id:key`, newest first), keep every old key until no stored value uses it; a backup taken earlier needs the keys that were current then.

## Taking a backup

With the Compose setup (PostgreSQL in a container named by Compose, usually `forgedeck-postgres-1`):

```sh
FORGEDECK_PG_CONTAINER=forgedeck-postgres-1 deploy/backup/backup.sh -d /var/backups/forgedeck -k 14
```

With a database you reach by URL (needs `pg_dump` of the same or a newer major version than the server):

```sh
FORGEDECK_DATABASE_URL='postgres://user:pass@db.example.com/forgedeck' deploy/backup/backup.sh -d /var/backups/forgedeck
```

It writes `forgedeck-<UTC time>.dump` (PostgreSQL custom format, readable only by its owner) and a `.sha256` next to it, writes through a temporary name so a failed dump never looks like a backup, and keeps the newest `-k` backups (default 14; `0` keeps everything).

**Schedule it.** For example a daily systemd timer or cron job:

```cron
17 3 * * *  FORGEDECK_PG_CONTAINER=forgedeck-postgres-1 /opt/forgedeck/deploy/backup/backup.sh -d /var/backups/forgedeck
```

**Copy it off the host.** A backup on the same disk is lost with the disk. Sync the directory to other storage (object storage, another machine). The files are not encrypted by the script; encrypt them in transit and at rest in the place you copy them to if it is not as trusted as the Control Plane.

What a restore gives back is the state **at the time of the backup**: anything that happened since (deployments, job history, audit events) is lost. The interval between backups is your data-loss window.

## Checking that a backup works

A backup you have never restored is a hope, not a backup. `--verify` restores into a temporary database next to the real one, prints what it found, and drops it. It changes nothing that exists, so it is safe to run on the live server:

```sh
FORGEDECK_PG_CONTAINER=forgedeck-postgres-1 deploy/backup/restore.sh --verify /var/backups/forgedeck/forgedeck-20261013T031700Z.dump
```

```text
restoring forgedeck-20261013T031700Z.dump into a temporary database (forgedeck_verify_…)...
restored. contents:
  users          3
  projects       2
  …
  migrations     21
OK: the backup restores cleanly. (The temporary database is dropped.)
```

It also refuses a file whose checksum no longer matches, which catches a truncated copy before PostgreSQL does. Run it on a schedule against the newest backup, and compare the counts with what you expect. **Do the full drill below at least once before you need it.**

## Restoring

Restore into a **new** environment when you can (a fresh host and an empty database): then the old one stays untouched while you check the new one. Restoring over the live database replaces it.

1. **Stop the Control Plane** (`docker compose stop api`), so nothing writes while the database is replaced. Agents and Runners keep retrying and carry on afterwards; services already deployed keep running.
2. **Have the key.** Put the `FORGEDECK_SECRET_KEYS` that was in use when the backup was taken into the environment of the new Control Plane.
3. **Restore.** For an empty database, create it first (`createdb forgedeck`), then:

   ```sh
   FORGEDECK_PG_CONTAINER=forgedeck-postgres-1 deploy/backup/restore.sh --yes /var/backups/forgedeck/forgedeck-20261013T031700Z.dump
   ```

   `--yes` is required because it replaces the database. It prints the same counts as `--verify`.
4. **Start the Control Plane.** It applies any migrations newer than the backup by itself (restoring an older backup into a newer ForgeDeck is supported; the reverse is not: do not restore a backup from a newer version into an older one).
5. **Check.** Sign in; open a project; look at the servers (they show *offline* until their Agents report in, usually within a minute); check Git connections and registries (they need the key); trigger a build.
6. **Webhooks and URLs.** If the Control Plane's public URL changed, update `FORGEDECK_PUBLIC_URL` and recreate the Git connections, which registered webhooks pointing at the old address.

### The drill

On a test machine: install ForgeDeck, restore a recent production backup with `--yes` and the production key, start it, and sign in. Then confirm what you rely on: a release you could roll back to is listed, a Git connection lists repositories (proof the key works), the audit log reaches back as far as you expect. Write down how long it took: that is your recovery time.

## What the scripts do not do

- They do not back up your applications' data or the container images.
- They do not encrypt the backup file.
- They do not copy it off the host or monitor that backups keep happening. Alert on the age of the newest file.
- Point-in-time recovery (continuous archiving) is out of scope for the MVP. If a day of data loss is too much, add PostgreSQL WAL archiving on top.

The scripts are tested against a real PostgreSQL: a backup and a verified restore, a restore that replaces data, a truncated file being refused (with and without its checksum), pruning, and the scripts refusing to guess a database (`FORGEDECK_TEST_DOCKER=1 go test ./deploy/backup/`).
