-- +goose Up
CREATE TABLE enrollment_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    server_name text NOT NULL CHECK (char_length(server_name) BETWEEN 1 AND 100),
    token_hash  bytea NOT NULL UNIQUE,
    created_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    server_id   uuid REFERENCES servers (id) ON DELETE SET NULL
);

CREATE INDEX enrollment_tokens_project_id_idx ON enrollment_tokens (project_id);

-- +goose Down
DROP TABLE enrollment_tokens;
