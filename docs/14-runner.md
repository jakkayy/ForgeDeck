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

- a Git clone URL and a token scoped to the repository (a fresh one-hour GitHub installation token limited to `contents: read` on that one repository, or the stored GitLab token);
- the registry host, username, and password;
- the image repository to push to, e.g. `ghcr.io/acme/shop-api`.

They are never written to job payloads or logs. If they cannot be minted (connection removed, provider down), the attempt fails at claim time with `build_credentials_unavailable` and the Runner is handed the next job instead.

## Build steps

```text
checkout → build → test → push
```

1. **Checkout** — a fresh temporary workspace; `git fetch --depth 1 <url> <commit>` and check out exactly that commit. The token reaches git as an `http.extraHeader` through `GIT_CONFIG_*` environment variables, not command arguments or the remote URL; redirects are not followed. Git may only use the protocols in `--git-protocols` (default `https`).
2. **Build** — `docker build` with BuildKit, using the application's Dockerfile and build context, tagged `<image>:<short sha>`.
3. **Test** — if the application has a test command, it runs inside the image just built: `docker run --rm --network none` with CPU, memory, and process limits. Tests never run directly on the build host. If the final image lacks test tooling, add a test stage to the Dockerfile and make it the target, or move the tests into a `RUN` step.
4. **Push** — only after tests pass: `docker login --password-stdin` into the job's own `DOCKER_CONFIG`, then push `<image>:<12-char sha>`. The pushed image's **digest** is the build's result and what deployments use:

```json
{ "commit_sha": "…", "image": "ghcr.io/acme/shop-api", "tag": "0123456789ab",
  "digest": "sha256:…", "image_ref": "ghcr.io/acme/shop-api@sha256:…" }
```

The image destination is read from the application when the build is claimed; an application without a registry and image fails with `no_image_destination`.

Build commands run with a minimal environment (`PATH`, a per-job `HOME` and `DOCKER_CONFIG`, and `DOCKER_HOST`/`DOCKER_TLS_VERIFY`/`DOCKER_CERT_PATH` if set), so they never see the Runner's credential.

Each job has a timeout (`--build-timeout`, default 20 minutes); a build that exceeds it fails with `build_timeout`. Test containers are limited by `--test-cpus` (default 2), `--test-memory` (default `2g`, swap capped to the same), and `--test-pids` (default 512). The workspace, the job's Docker config directory, and the image tags the build created on the build host are deleted afterwards, success or not; the pushed image stays in the registry.

## Isolation and threat model

Repository code, Dockerfiles, and test commands are untrusted.

| Threat | Mitigation |
|---|---|
| Build steps reading the host | BuildKit runs `RUN` steps in containers without host mounts. The Runner never mounts the Docker socket into a build or test container. |
| Tests attacking the network | Test containers run with `--network none`. |
| Resource exhaustion | Test containers get `--cpus`, `--memory`, `--pids-limit`; every job has a timeout. Configure BuildKit's own limits on the build host for build steps. |
| Credential theft | Credentials are per job and short-lived where the provider allows; docker login uses a per-job config directory; build commands do not inherit the Runner's environment. Every log line is scrubbed of the job's secret values and their HTTP Basic (base64) forms, replaced with `********`, before it leaves the Runner. Redaction cannot catch a secret a tool transforms in other ways, so do not echo credentials in build scripts. |
| Reaching production | The build host has no Agent and no production Docker access. Images reach production only through the registry. |

Container isolation is not a perfect boundary for hostile code. Run Runners on hosts that hold nothing else of value, and do not build untrusted forks on the same host as trusted code.
