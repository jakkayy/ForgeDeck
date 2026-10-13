# 13 — Git Providers

How a ForgeDeck installation connects to GitHub and GitLab. Every project manages its own connections; credentials are encrypted with `FORGEDECK_SECRET_KEYS` and never returned by the API.

`FORGEDECK_PUBLIC_URL` must be set to the address users and providers reach ForgeDeck at (for example `https://forgedeck.example.com`). Callback and webhook URLs below are relative to it.

## GitHub (GitHub App)

ForgeDeck uses one GitHub App per installation. Projects connect by installing that App on a user or organization and choosing repositories. Installation tokens are short-lived (one hour) and limited to the selected repositories.

### Create the App

In GitHub: **Settings → Developer settings → GitHub Apps → New GitHub App** (or the organization's equivalent).

| Setting | Value |
|---|---|
| Homepage URL | `FORGEDECK_PUBLIC_URL` |
| Callback URL | `{FORGEDECK_PUBLIC_URL}/api/v1/git/github/setup` |
| Request user authorization (OAuth) during installation | **On** (required, see below) |
| Setup URL | leave empty (the callback URL is used) |
| Webhook | Active, URL `{FORGEDECK_PUBLIC_URL}/api/v1/webhooks/github`, a random secret |
| Repository permissions | Contents: Read-only, Metadata: Read-only |
| Subscribe to events | Push |
| Where can this App be installed | Any account, or only this account |

Then generate a private key and configure ForgeDeck:

```text
FORGEDECK_GITHUB_APP_ID=<App ID>
FORGEDECK_GITHUB_APP_SLUG=<app-name-in-the-URL>
FORGEDECK_GITHUB_CLIENT_ID=<Client ID>
FORGEDECK_GITHUB_CLIENT_SECRET=<Client secret>
FORGEDECK_GITHUB_APP_PRIVATE_KEY_FILE=/run/secrets/github-app.pem
FORGEDECK_GITHUB_WEBHOOK_SECRET=<webhook secret>
```

For GitHub Enterprise Server also set `FORGEDECK_GITHUB_URL` and `FORGEDECK_GITHUB_API_URL`.

### Connect flow and why OAuth is required

1. A project admin clicks **Connect GitHub**. ForgeDeck returns the App's install URL with a `state` sealed by the keyring: it names the project and user and expires after 15 minutes.
2. GitHub redirects back to the callback with `installation_id`, `code`, and `state`.
3. ForgeDeck checks the state, exchanges `code` for the GitHub user's token, and confirms through `GET /user/installations` that this user can access `installation_id`. It then re-checks the user's role in the project and stores the connection.

`installation_id` is a guessable number in the URL. Without step 3, anyone could paste another organization's installation ID into the callback and gain access to its repositories through the App. The callback works without the session cookie (GitHub's redirect is cross-site, and the cookie is `SameSite=Strict`), so the sealed state is what identifies the user.

## GitLab (access token)

Works with gitlab.com and self-hosted GitLab. Each connection holds one **project or group access token** with:

- Scope: `api` (read repositories and register the ForgeDeck webhook)
- Role: Maintainer (needed to manage webhooks)

A project admin pastes the token and the GitLab address in **Project → Git**. ForgeDeck checks with GitLab that the token is active and has the `api` scope, then stores it encrypted together with a generated webhook secret.

When an application is created from a GitLab repository, ForgeDeck registers a push webhook on that GitLab project pointing to `{FORGEDECK_PUBLIC_URL}/api/v1/webhooks/gitlab/{connectionID}`, with the connection's secret as the token. Deleting the application removes the hook. GitHub needs no per-repository hook: the App's webhook covers every installed repository.

### Self-hosted GitLab on a private network

The GitLab address is user input, so ForgeDeck connects only to public addresses by default, checked after DNS resolution. Loopback, link-local (including cloud metadata at `169.254.169.254`), and private ranges are refused, and redirects are not followed. If your GitLab is on an internal network, the operator can allow private addresses:

```text
FORGEDECK_ALLOW_PRIVATE_GIT_HOSTS=true
```

This also permits `http://` GitLab addresses, for internal instances without TLS. Loopback and link-local addresses stay blocked.
