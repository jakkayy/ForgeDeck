-- +goose Up
CREATE TABLE jobs (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    type             text NOT NULL,
    worker_kind      text NOT NULL CHECK (worker_kind IN ('agent', 'runner')),
    -- Agent jobs run on exactly one server; deleting the server drops them.
    target_server_id uuid REFERENCES servers (id) ON DELETE CASCADE,
    status           text NOT NULL DEFAULT 'queued'
                     CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    payload          jsonb NOT NULL,
    result           jsonb,
    error_code       text,
    error_message    text,
    attempt_count    integer NOT NULL DEFAULT 0,
    max_attempts     integer NOT NULL DEFAULT 1 CHECK (max_attempts >= 1),
    lease_owner      text,
    lease_expires_at timestamptz,
    idempotency_key  text,
    created_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    started_at       timestamptz,
    finished_at      timestamptz,
    CHECK (
        (worker_kind = 'agent' AND target_server_id IS NOT NULL)
        OR (worker_kind = 'runner' AND target_server_id IS NULL)
    ),
    UNIQUE (project_id, idempotency_key)
);

CREATE INDEX jobs_agent_claim_idx ON jobs (target_server_id, created_at)
    WHERE status = 'queued' AND worker_kind = 'agent';
CREATE INDEX jobs_runner_claim_idx ON jobs (created_at)
    WHERE status = 'queued' AND worker_kind = 'runner';
CREATE INDEX jobs_lease_expiry_idx ON jobs (lease_expires_at)
    WHERE status = 'running';
CREATE INDEX jobs_project_history_idx ON jobs (project_id, created_at DESC);

CREATE TABLE job_attempts (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id         uuid NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    attempt_number integer NOT NULL,
    worker_id      text NOT NULL,
    status         text NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled', 'lost')),
    started_at     timestamptz NOT NULL,
    finished_at    timestamptz,
    error_code     text,
    error_summary  text,
    UNIQUE (job_id, attempt_number)
);

CREATE TABLE job_log_chunks (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id          uuid NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    attempt_number  integer NOT NULL,
    sequence_number bigint NOT NULL,
    stream          text NOT NULL CHECK (stream IN ('stdout', 'stderr', 'system')),
    content         text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- Agents retry batches; duplicates are dropped by this key.
    UNIQUE (job_id, attempt_number, sequence_number)
);

-- +goose Down
DROP TABLE job_log_chunks;
DROP TABLE job_attempts;
DROP TABLE jobs;
