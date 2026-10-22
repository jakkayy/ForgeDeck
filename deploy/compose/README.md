# ForgeDeck in production (Docker Compose)

One host runs three containers: the **Control Plane** (API and dashboard in one binary), **PostgreSQL**, and a **Caddy** reverse proxy that terminates TLS and gets its certificate from Let's Encrypt by itself. Servers that run applications (Agents) and the build host (Runner) are separate machines; see the end.

```text
 browser, Agents, Runners, Git webhooks
              │ HTTPS 443  (HTTP 80 redirects)
        ┌─────▼─────┐         ┌───────────────┐  internal network, no route out
        │   Caddy   │────────►│ Control Plane │────────────► PostgreSQL
        └───────────┘  web    └───────────────┘     db
```

Only Caddy publishes ports. The Control Plane and the database are reachable from inside the stack only; the database network has no route to the internet.

## You need

- A host with Docker and the Docker Compose plugin, and **ports 80 and 443 reachable from the internet** (Caddy proves domain ownership over them).
- A **DNS name** pointing at the host.
- About 1 GB of RAM for the stack.

## Install

```sh
git clone https://github.com/jakkayy/ForgeDeck && cd ForgeDeck/deploy/compose
cp .env.example .env
```

Edit `.env`:

| Setting | What to put |
|---|---|
| `FORGEDECK_DOMAIN` | the DNS name, such as `forgedeck.example.com` |
| `POSTGRES_PASSWORD` | `openssl rand -hex 24` (letters and digits only) |
| `FORGEDECK_SECRET_KEYS` | `echo "v1:$(openssl rand -base64 32)"` |

**Copy `FORGEDECK_SECRET_KEYS` somewhere safe now, away from this host.** It encrypts stored Git tokens, registry passwords and project secrets, and it is neither in the database nor in backups: without it those values are unreadable after a restore ([16](../../docs/16-backup-restore.md)).

```sh
docker compose up -d --build        # builds the image from this repository, starts everything
docker compose ps                   # wait for api and postgres to be "healthy"
```

Open `https://<your domain>`: a new installation shows **Create the first account**. Register there, straight away; that account owns the installation and the page closes for good once it exists. The first start also applies the database migrations.

**Until you register, whoever reaches the page first takes the installation.** If the host is reachable from the internet (a public domain), set `FORGEDECK_DISABLE_WEB_SETUP=true` in `.env` first and create the account on the host instead (it asks for the password twice):

```sh
docker compose exec api /forgedeck-api create-user --email you@example.com
```

More users are made with that command, and added to projects under *Members*.

## What is configured for you

- **TLS and headers.** HTTPS with HTTP/2, plain HTTP redirected to HTTPS; the application adds HSTS, a strict Content-Security-Policy and the other security headers ([07](../../docs/07-security.md)); secure `__Host-` session cookies.
- **Real client addresses.** The proxy has a fixed address on the stack's network and the Control Plane trusts `X-Forwarded-For` from that address only (`FORGEDECK_TRUSTED_PROXIES`), so rate limits and the audit log see each client, and a client cannot forge its address.
- **A hardened container.** The Control Plane image is a static binary with no shell or package manager, runs as a non-root user with a read-only filesystem, no Linux capabilities and no new privileges, and checks its own health.
- **Live logs.** The proxy does not buffer Server-Sent Events, and allows Agents' 25-second long-polls.

## Day-to-day

```sh
docker compose logs -f api          # structured JSON; each request has request_id and client_ip
docker compose ps
docker compose restart api
```

- **Back up** the database on a schedule and **check** that the backups restore: [16](../../docs/16-backup-restore.md). Here the database container is `forgedeck-prod-postgres-1`:
  `FORGEDECK_PG_CONTAINER=forgedeck-prod-postgres-1 ../backup/backup.sh -d /var/backups/forgedeck`
- **Upgrade:** back up and verify first, then check out the new version and run `docker compose up -d --build`; migrations run on start. The order, what happens to work in progress, and how to go back are in [17](../../docs/17-upgrade-guide.md).
- **The GitHub App and other optional settings** go in `.env` (see the commented lines in `.env.example`); then `docker compose up -d`.

## Connecting machines

- **Servers that run applications:** in the dashboard, *Servers → Add server*, then run the Agent there (it needs Docker and the Docker Compose plugin) and enroll it against `https://<your domain>`. The Agent connects outward over HTTPS; the server needs no inbound port.
- **The build host:** a separate machine, never this one or a production server: [`deploy/runner`](../runner/README.md).

## Trying it on one machine

To see the stack without a public domain, use `FORGEDECK_DOMAIN=localhost`: Caddy then uses its own local certificate authority (your browser will warn, and `curl` needs `-k`). If ports 80 and 443 are taken, also set:

```sh
FORGEDECK_HTTP_PORT=8080
FORGEDECK_HTTPS_PORT=8443
FORGEDECK_PUBLIC_URL=https://localhost:8443
```

## Do not mix it with the development stack

This stack's Compose project is named **`forgedeck-prod`** so that its containers and volumes cannot collide with the development `docker-compose.yml` in the repository root (project `forgedeck`). Do not run `docker compose down --volumes` in a directory whose project name you are unsure of: it deletes that project's database.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `docker compose up` stops with `set POSTGRES_PASSWORD…` | a required setting is missing in `.env` |
| The browser shows a certificate error on a real domain | DNS does not point at this host yet, or ports 80/443 are blocked; see `docker compose logs proxy` |
| `api` keeps restarting | `docker compose logs api` names the setting that is wrong (for example a malformed `FORGEDECK_SECRET_KEYS`) |
| Agents cannot connect | the Agent must use `https://<your domain>`; check `docker compose logs proxy` |
| Every client appears as one address in the logs | the proxy is not at 172.30.0.2, or `FORGEDECK_TRUSTED_PROXIES` was changed |

The Compose file is checked in CI-style tests (`FORGEDECK_TEST_DOCKER=1 go test ./deploy/compose/`): only the proxy publishes ports, the database network has no route out, the Control Plane is hardened, the trusted proxy is the proxy's fixed address, required settings are enforced, and the project name does not clash with the development stack.
