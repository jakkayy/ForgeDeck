-- +goose Up
-- Build workers for the whole installation. Created by an operator with
-- `forgedeck-api create-runner`; deleting the row revokes the credential.
CREATE TABLE runners (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL UNIQUE CHECK (name ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    credential_hash bytea NOT NULL UNIQUE,
    version         text,
    last_seen_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE runners;
