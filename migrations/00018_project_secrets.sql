-- +goose Up
-- Write-only project secrets delivered at deploy time; AES-GCM ciphertext bound to the row.
CREATE TABLE project_secrets (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name       text NOT NULL CHECK (name ~ '^[A-Z_][A-Z0-9_]{0,63}$'),
    ciphertext bytea NOT NULL,
    key_id     text NOT NULL,
    version    integer NOT NULL DEFAULT 1,
    created_by uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

-- +goose Down
DROP TABLE project_secrets;
