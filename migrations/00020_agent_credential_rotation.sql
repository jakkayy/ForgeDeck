-- +goose Up
-- The replaced credential keeps working for a grace period so a lost rotation response cannot lock the Agent out.
ALTER TABLE agents
    ADD COLUMN previous_credential_hash bytea UNIQUE,
    ADD COLUMN previous_expires_at      timestamptz,
    ADD COLUMN credential_rotated_at    timestamptz NOT NULL DEFAULT now(),
    -- Set when an admin asks for a rotation; the next heartbeat tells the Agent.
    ADD COLUMN rotation_requested_at    timestamptz,
    ADD CONSTRAINT agents_previous_credential_check
        CHECK ((previous_credential_hash IS NULL) = (previous_expires_at IS NULL));

-- +goose Down
ALTER TABLE agents
    DROP CONSTRAINT agents_previous_credential_check,
    DROP COLUMN previous_credential_hash,
    DROP COLUMN previous_expires_at,
    DROP COLUMN credential_rotated_at,
    DROP COLUMN rotation_requested_at;
