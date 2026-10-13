# 14 — Runner

The Runner builds images from commits. It runs on a **dedicated build host**, never on a production server, and pulls work from the Control Plane over HTTPS like the Agent does.

## Identity

Runners belong to the whole ForgeDeck installation, not to a project. An operator creates one on the Control Plane host:

```text
forgedeck-api create-runner --name build-01
```

This prints the Runner credential once (only its hash is stored). The operator puts it on the build host as `FORGEDECK_RUNNER_CREDENTIAL` or in a file read by `forgedeck-runner run --credential-file`. There is no web enrollment step: whoever installs a Runner already has access to the Control Plane host. Removing the row (`forgedeck-api delete-runner`) revokes it.

## Protocol

The Runner speaks the Agent protocol ([05](05-agent-protocol.md)) under `/api/v1/runner/`:

| Endpoint | Same as Agent |
|---|---|
| `POST /runner/heartbeat` | `/agent/heartbeat` |
| `POST /runner/jobs/claim` | long-poll claim; returns jobs with `worker_kind = 'runner'` |
| `POST /runner/jobs/{id}/heartbeat` | lease renewal |
| `POST /runner/jobs/{id}/events` | log batches |
| `POST /runner/jobs/{id}/complete` | outcome |

Lease ownership is checked by worker kind and worker ID (and, for Agents, the target server), so one implementation serves both.

A claimed `build` job carries **build credentials** minted at claim time:

- a Git clone URL and a token scoped to the repository (a fresh one-hour GitHub installation token, or the stored GitLab token);
- the registry host, username, and password;
- the image repository to push to, e.g. `ghcr.io/acme/shop-api`.

They are never written to job payloads or logs.

## Build steps

```text
checkout → build → test → push
```

1. **Checkout** — a fresh temporary workspace; `git fetch --depth 1 <url> <commit>` and check out exactly that commit. The token reaches git through `GIT_CONFIG_*` environment variables, not command arguments.
2. **Build** — `docker build` with BuildKit, using the application's Dockerfile and build context, tagged `<image>:<short sha>`.
3. **Test** — if the application has a test command, it runs inside the image just built: `docker run --rm --network none` with CPU, memory, and process limits. Tests never run directly on the build host. If the final image lacks test tooling, add a test stage to the Dockerfile and make it the target, or move the tests into a `RUN` step.
4. **Push** — only after tests pass. The pushed image's **digest** (`repo@sha256:…`) is the build's result and what deployments use.

Each job has a timeout (default 20 minutes). The workspace and the job's Docker config directory are deleted afterwards, success or not.

## Isolation and threat model

Repository code, Dockerfiles, and test commands are untrusted.

| Threat | Mitigation |
|---|---|
| Build steps reading the host | BuildKit runs `RUN` steps in containers without host mounts. The Runner never mounts the Docker socket into a build or test container. |
| Tests attacking the network | Test containers run with `--network none`. |
| Resource exhaustion | Test containers get `--cpus`, `--memory`, `--pids-limit`; every job has a timeout. Configure BuildKit's own limits on the build host for build steps. |
| Credential theft | Credentials are per job and short-lived where the provider allows; docker login uses a per-job config directory; known secret values are replaced with `********` in logs. |
| Reaching production | The build host has no Agent and no production Docker access. Images reach production only through the registry. |

Container isolation is not a perfect boundary for hostile code. Run Runners on hosts that hold nothing else of value, and do not build untrusted forks on the same host as trusted code.
