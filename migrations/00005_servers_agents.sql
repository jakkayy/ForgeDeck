-- +goose Up
CREATE TABLE servers (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- RESTRICT: a project cannot be deleted while servers are still attached.
    project_id    uuid NOT NULL REFERENCES projects (id) ON DELETE RESTRICT,
    name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    agent_version text,
    last_seen_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

-- One Agent identity per server. Deleting the server deletes the Agent,
-- which revokes its credential.
CREATE TABLE agents (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       uuid NOT NULL UNIQUE REFERENCES servers (id) ON DELETE CASCADE,
    credential_hash bytea NOT NULL UNIQUE,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE agents;
DROP TABLE servers;
