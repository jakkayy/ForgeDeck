# Security policy

ForgeDeck holds Git tokens, registry passwords and project secrets, and its Agent controls Docker on your servers, so please report vulnerabilities privately.

## Reporting a vulnerability

Use GitHub's **Report a vulnerability** button on the repository's *Security* tab (private vulnerability reporting). Please do not open a public issue, and do not include real tokens, keys or `.env` values in a report.

Include the version (`forgedeck-api version`), what you did, what you expected and what happened. Reports are handled on a best-effort basis by a single maintainer.

## Supported versions

Only the latest release receives fixes. See the [upgrade guide](docs/17-upgrade-guide.md).

## Where the threats and mitigations are described

[docs/07-security.md](docs/07-security.md) lists the threat model, the trust boundaries and the production checklist. Its main points: the Runner executes untrusted repository code and belongs on a separate build host, and access to a server's Docker socket is root-equivalent on that server.
