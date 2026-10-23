# ForgeDeck Runner deployment

The Runner builds images and runs tests for untrusted repository code, so it
belongs on a **dedicated build host** that holds nothing else of value and has
no Agent and no production Docker access. Read [docs/14-runner.md](../../docs/14-runner.md)
for the protocol and threat model.

## 1. Create the Runner in the dashboard

Sign in as the installation's **administrator** (the first account) and open **Runners → Add Runner** (or run `forgedeck-api create-runner --name build-01` on the Control Plane host). The credential (`fdr_…`) is shown **once**; Runners belong to the whole installation, so only an administrator can create them. Deleting a Runner revokes its credential.

## 2. Install on the build host

Requirements: Linux with systemd, Docker Engine with BuildKit, and `git`.

```sh
curl -fsSL https://github.com/jakkayy/ForgeDeck/releases/latest/download/install-runner.sh -o install-runner.sh
sudo bash install-runner.sh --url https://forgedeck.example.com
```

The installer downloads the Runner for this CPU (amd64 or arm64) from the same release and checks it against the release's `SHA256SUMS` (that catches a corrupted download, not a compromised release), asks for the credential at a hidden prompt (or reads `FORGEDECK_RUNNER_CREDENTIAL`), and writes `/etc/forgedeck-runner/runner.env` readable only by root and the Runner. It also creates the `forgedeck-runner` user and installs the systemd unit, then prints the Docker setup steps below. `--version v0.1.0` picks a release, `--binary PATH` installs a binary you built (`make build-runner`), and running it again upgrades the binary and keeps `runner.env`. `--dry-run` shows what it would do; `--uninstall` removes the service and binary.

## 3. Give the Runner its own Docker daemon

The Runner drives Docker through the `docker` CLI. Adding its user to the
`docker` group would give anything that escapes a build root on the host.
Prefer **rootless Docker** for the `forgedeck-runner` user: container root maps
to an unprivileged user, so even a breakout from a build step stays unprivileged.
The unit points `DOCKER_HOST` at that user's socket. Rootless mode cannot
publish privileged ports, which builds do not need.

If you must use the system daemon, remove the `DOCKER_HOST` line from the unit
and understand that the Runner then has root-equivalent access to the host.

## 4. Start and check

```sh
sudo systemctl enable --now forgedeck-runner
journalctl -u forgedeck-runner -f
```

Look for `runner started`. The Control Plane marks the Runner seen on its first
heartbeat. Then press **Build** on an application with a registry and image.

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
