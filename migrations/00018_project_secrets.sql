-- +goose Up
-- Named, write-only values a project's services receive at deployment time
-- (docs/15-deployments.md). The value is AES-GCM ciphertext bound to the row;
-- key_id names the key that sealed it.
CREATE TABLE project_secrets (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name       text NOT NULL CHECK (name ~ '^[A-Z_][A-Z0-9_]{0,63}$'),
    ciphertext bytea NOT NULL,
    key_id     text NOT NULL,
    -- Incremented each time the value is replaced.
    version    integer NOT NULL DEFAULT 1,
    created_by uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

-- +goose Down
DROP TABLE project_secrets;
