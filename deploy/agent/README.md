# The ForgeDeck Agent on a server

The Agent runs on each Docker host that ForgeDeck manages. It connects **outward** to the Control Plane over HTTPS, so the server needs no inbound port and can sit behind NAT. It reports the host's health and containers, and carries out the deployments and container actions it is given.

## Requirements

- Linux with **systemd**, **Docker Engine** and the **Docker Compose plugin** (`docker compose version` works).
- Outbound HTTPS to your ForgeDeck, and to the container registry the images come from.
- Root access to install.

## Install

1. In the dashboard open **Servers → Add server**, name the server and copy the **enrollment token**. It is valid for a short time and works once.
2. On the server, download the installer from the latest release and run it (replace the URL with your ForgeDeck's address):

```sh
curl -fsSL https://github.com/jakkayy/ForgeDeck/releases/latest/download/install-agent.sh -o install-agent.sh
sudo bash install-agent.sh --url https://forgedeck.example.com
```

It downloads the Agent for this CPU (amd64 or arm64) from the same release, checks it against the release's `SHA256SUMS`, and asks for the token at a hidden prompt. The checksums come from the same place as the binary, so this catches a corrupted download, not a compromised release. Read the script before you run it if you want to be sure what it does.

To install a specific version, add `--version v0.1.0`. To install a binary you built yourself (`make build-agent` produces `bin/forgedeck-agent`), add `--binary ./forgedeck-agent` instead; the token can also be given as `FORGEDECK_ENROLLMENT_TOKEN=<token>` in the environment. The token is never taken as an argument, so it does not land in shell history or `ps`.

The installer creates a `forgedeck-agent` user (member of the `docker` group), installs the binary to `/usr/local/bin`, installs and starts the `forgedeck-agent` systemd service, and enrolls. The server shows as **online** in the dashboard within a minute. Check it with:

```sh
systemctl status forgedeck-agent
journalctl -u forgedeck-agent -f
```

## Upgrade

Run the installer again (it downloads the latest release unless you pass `--version` or `--binary`). It replaces the binary and the unit and restarts the service; it recognizes the existing identity and does not enroll again.

```sh
sudo bash install-agent.sh
```

A deployment running during the restart is picked up again by the Control Plane after its lease expires (about a minute).

## Remove

```sh
sudo bash install-agent.sh --uninstall            # stop and remove the service and binary; keeps the identity and user
sudo bash install-agent.sh --uninstall --purge    # also delete the identity and the user
```

Containers that ForgeDeck deployed keep running; nothing here stops them. Remove the server in the dashboard too: that revokes the Agent's credential.

To enroll the same host again, remove it in the dashboard, `--purge` it here, and install with a new token.

## What the Agent is allowed to do, and what protects the host

**Be clear about the privilege.** Access to the Docker socket is root-equivalent on that host: anything that can talk to Docker can start a container that mounts the whole filesystem. That is what the Agent needs in order to deploy, so a server's Agent can do anything to that server. Run the Agent only on hosts that you are willing to let ForgeDeck manage.

What limits the damage of a *flaw in the Agent itself*:

- It runs as its own unprivileged user, not as root, and the unit sets `NoNewPrivileges`, a read-only view of the whole filesystem except its own state directory (`ProtectSystem=strict`), no access to home directories, a private `/tmp`, and read-only kernel settings.
- Its credential and deployment files live in `/var/lib/forgedeck-agent` (mode 0700, readable only by the Agent). The credential is rotated automatically every 90 days, or on demand from the server's page ([05](../../docs/05-agent-protocol.md)).
- It accepts only a fixed set of job types and re-validates every deployment configuration itself; the Control Plane cannot make it run an arbitrary command. A deployment never mounts a host path or the Docker socket into your containers.

Registry passwords for pulls are written to a per-job Docker config that is deleted afterwards.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `Docker Compose plugin is required` | install the `docker-compose-plugin` package, then run the installer again |
| `there is no 'docker' group` | Docker is not installed, or was installed in a way that does not create the group |
| Enrollment is refused with 401 | the token expired or was already used; create a new one |
| The server stays *waiting for Agent* | `journalctl -u forgedeck-agent`; the host must reach `https://<your ForgeDeck>` |
| *Docker not reachable* in the dashboard | the `forgedeck-agent` user cannot use the Docker socket: it must be in the `docker` group (the installer adds it); after changing groups, `systemctl restart forgedeck-agent` |
| A deployment fails with `compose_unavailable` | the Docker Compose plugin is missing on this host |

`install.sh --dry-run` prints what it would do without changing anything.

The installer is tested end to end in a container against a real Control Plane (`FORGEDECK_TEST_DOCKER=1 go test ./deploy/agent/`): user and permissions, enrolling once, upgrading without enrolling again, uninstall and purge, and its refusals. systemd itself is not run in that test; `systemd-analyze verify` checks the unit.
