# ForgeDeck Runner deployment

The Runner builds images and runs tests for untrusted repository code, so it
belongs on a **dedicated build host** that holds nothing else of value and has
no Agent and no production Docker access. Read [docs/14-runner.md](../../docs/14-runner.md)
for the protocol and threat model.

## 1. Create the Runner in the dashboard

Sign in as the installation's **administrator** (the first account) and open **Runners → Add Runner** (or run `forgedeck-api create-runner --name build-01` on the Control Plane host). The credential (`fdr_…`) is shown **once**; Runners belong to the whole installation, so only an administrator can create them. Deleting a Runner revokes its credential.

## 2. Install on the build host

Requirements: Linux with systemd, `git`, and Debian or Ubuntu with [Docker's apt repository](https://docs.docker.com/engine/install/ubuntu/) (the installer adds rootless Docker from it). Use the command the dashboard shows after **Add Runner**; it has the credential in it:

```sh
curl -fsSL https://github.com/jakkayy/ForgeDeck/releases/latest/download/install-runner.sh -o install-runner.sh
sudo FORGEDECK_RUNNER_CREDENTIAL=fdr_... bash install-runner.sh --url https://forgedeck.example.com
```

That is the whole installation. The installer:

- downloads the Runner for this CPU (amd64 or arm64) from the same release and checks it against the release's `SHA256SUMS` (that catches a corrupted download, not a compromised release);
- creates the `forgedeck-runner` user and writes `/etc/forgedeck-runner/runner.env`, readable only by root and the Runner;
- installs **rootless Docker** for that user (`docker-ce-rootless-extras` and `uidmap` from apt, a non-overlapping `subuid`/`subgid` range, `loginctl enable-linger`), see below;
- installs and starts the systemd unit, then checks that it stays up and tells you if it does not.

Installing those packages upgrades the Docker packages the host already has, which can restart its Docker daemon, so use a host built for this. The credential is not shown in `ps`, but the command puts it in your shell history: delete the Runner in the dashboard to revoke it. Without `FORGEDECK_RUNNER_CREDENTIAL` the installer asks for it at a hidden prompt. Giving it a credential again replaces the stored one, which is how you recover from a wrong or revoked credential. `--version v0.1.0` picks a release, `--binary PATH` installs a binary you built (`make build-runner`), `--no-docker-setup` leaves Docker to you, and running it again upgrades the binary and keeps `runner.env`. `--dry-run` shows what it would do; `--uninstall` removes the service and binary.

## 3. Why the Runner gets its own Docker daemon

The Runner drives Docker through the `docker` CLI. Adding its user to the
`docker` group would give anything that escapes a build root on the host.
**Rootless Docker** for the `forgedeck-runner` user means container root maps
to an unprivileged user, so even a breakout from a build step stays unprivileged.
The unit points `DOCKER_HOST` at that user's socket. Rootless mode cannot
publish privileged ports, which builds do not need.

If the installer cannot set it up (a distribution without Docker's apt repository), install
rootless Docker for the user by hand (`dockerd-rootless-setuptool.sh install` with `loginctl enable-linger`)
and run `sudo systemctl enable --now forgedeck-runner`. If you must use the system daemon, remove
the `DOCKER_HOST` line from the unit and understand that the Runner then has root-equivalent access to the host.

## 4. Check

```sh
journalctl -u forgedeck-runner -f
```

Look for `runner started`. The Control Plane marks the Runner seen on its first
heartbeat. A `401` means the credential is wrong or the Runner was deleted: add a new Runner and run the command again. Then press **Build** on an application with a registry and image.

## Tuning

`runner.env` sets the build timeout and test-container limits
(`FORGEDECK_BUILD_TIMEOUT`, `FORGEDECK_TEST_CPUS`, `FORGEDECK_TEST_MEMORY`,
`FORGEDECK_TEST_PIDS`). BuildKit's own CPU and memory limits are configured on
the build host's Docker daemon, not by the Runner.

## Hardening already in the unit

A dedicated unprivileged user, `NoNewPrivileges`, a read-only filesystem except
`/var/lib/forgedeck-runner`, a private `/tmp`, no access to home directories,
and a credential readable only by root and the Runner's group.
Build commands run with a minimal environment and never see the Runner's credential.
