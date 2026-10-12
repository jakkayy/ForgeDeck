-- +goose Up
-- A project's link to a Git provider. GitHub uses the ForgeDeck GitHub App
-- (installation ID; the app's own key and webhook secret live in config).
-- GitLab uses a project access token and a per-connection webhook secret.
-- Credentials are AES-GCM ciphertext; *_key_id names the key that sealed them.
CREATE TABLE git_connections (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id            uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    provider              text NOT NULL CHECK (provider IN ('github', 'gitlab')),
    display_name          text NOT NULL,
    base_url              text NOT NULL,
    installation_id       bigint,
    token_ciphertext      bytea,
    token_key_id          text,
    webhook_ciphertext    bytea,
    webhook_key_id        text,
    created_by            uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (provider = 'github' AND installation_id IS NOT NULL AND token_ciphertext IS NULL)
        OR (provider = 'gitlab' AND token_ciphertext IS NOT NULL AND token_key_id IS NOT NULL
            AND webhook_ciphertext IS NOT NULL AND webhook_key_id IS NOT NULL)
    ),
    UNIQUE (project_id, installation_id)
);

CREATE INDEX git_connections_project_id_idx ON git_connections (project_id);
-- GitHub webhooks are app-wide; events are routed by installation ID.
CREATE INDEX git_connections_installation_idx ON git_connections (installation_id)
    WHERE installation_id IS NOT NULL;

-- +goose Down
DROP TABLE git_connections;
