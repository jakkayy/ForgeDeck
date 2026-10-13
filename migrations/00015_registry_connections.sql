-- +goose Up
-- Container registries a project's Runners push images to. The password
-- (or token) is AES-GCM ciphertext; password_key_id names the sealing key.
CREATE TABLE registry_connections (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name                text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    host                text NOT NULL,
    username            text NOT NULL,
    password_ciphertext bytea NOT NULL,
    password_key_id     text NOT NULL,
    created_by          uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

-- Where an application's images are pushed: <registry host>/<image_repository>.
-- RESTRICT: a registry cannot be removed while applications push to it.
ALTER TABLE applications
    ADD COLUMN registry_connection_id uuid REFERENCES registry_connections (id) ON DELETE RESTRICT,
    ADD COLUMN image_repository text,
    ADD CONSTRAINT applications_image_check
        CHECK ((registry_connection_id IS NULL) = (image_repository IS NULL));

CREATE INDEX applications_registry_idx ON applications (registry_connection_id)
    WHERE registry_connection_id IS NOT NULL;

-- +goose Down
ALTER TABLE applications DROP COLUMN registry_connection_id, DROP COLUMN image_repository;
DROP TABLE registry_connections;
