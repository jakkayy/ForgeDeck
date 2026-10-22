-- +goose Up
-- One attempt to put an image digest into an environment; failures are kept.
CREATE TABLE deployments (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    environment_id      uuid NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    -- The deploy_image job that carries it out. Kept if the job is pruned.
    job_id              uuid UNIQUE REFERENCES jobs (id) ON DELETE SET NULL,
    -- no CHECK on the source: both are SET NULL on delete, which must not block it
    source_build_job_id uuid REFERENCES jobs (id) ON DELETE SET NULL,
    source_release_id   uuid,
    commit_sha          text NOT NULL,
    -- registry/repo@sha256:…; a mutable tag is never deployed.
    image_ref           text NOT NULL CHECK (image_ref ~ '@sha256:[0-9a-f]{64}$'),
    -- the configuration applied (a release's snapshot when rolling back)
    config              jsonb NOT NULL,
    status              text NOT NULL DEFAULT 'queued'
                        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    stage               text CHECK (stage IN ('pull', 'start', 'health_check')),
    error_code          text,
    error_message       text,
    created_by          uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    started_at          timestamptz,
    finished_at         timestamptz
);

CREATE INDEX deployments_environment_idx ON deployments (environment_id, created_at DESC, id DESC);

-- A release is a deployment that passed its health check; immutable, restored by rollback.
CREATE TABLE releases (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    deployment_id  uuid NOT NULL UNIQUE REFERENCES deployments (id) ON DELETE CASCADE,
    commit_sha     text NOT NULL,
    image_ref      text NOT NULL CHECK (image_ref ~ '@sha256:[0-9a-f]{64}$'),
    config         jsonb NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX releases_environment_idx ON releases (environment_id, created_at DESC, id DESC);

ALTER TABLE deployments
    ADD CONSTRAINT deployments_source_release_fk
    FOREIGN KEY (source_release_id) REFERENCES releases (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE deployments DROP CONSTRAINT deployments_source_release_fk;
DROP TABLE releases;
DROP TABLE deployments;
