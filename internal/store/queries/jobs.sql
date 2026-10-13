-- name: InsertJob :one
-- Returns no row when a job with the same idempotency key already exists.
INSERT INTO jobs (project_id, application_id, type, worker_kind, target_server_id, payload, max_attempts, idempotency_key, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (project_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: GetJobByIdempotencyKey :one
SELECT * FROM jobs WHERE project_id = $1 AND idempotency_key = $2;

-- name: NotifyWorkers :exec
SELECT pg_notify(@channel::text, @payload::text);

-- name: ClaimAgentJob :one
WITH next AS (
    SELECT queued.id FROM jobs AS queued
    WHERE queued.status = 'queued'
      AND queued.worker_kind = 'agent'
      AND queued.target_server_id = @server_id::uuid
    ORDER BY queued.created_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE jobs
SET status           = 'running',
    attempt_count    = attempt_count + 1,
    lease_owner      = @worker_id::text,
    lease_expires_at = @lease_expires_at::timestamptz,
    started_at       = COALESCE(started_at, @now::timestamptz)
FROM next
WHERE jobs.id = next.id
RETURNING jobs.*;

-- name: InsertJobAttempt :exec
INSERT INTO job_attempts (job_id, attempt_number, worker_id, status, started_at)
VALUES ($1, $2, $3, 'running', $4);

-- name: RenewJobLease :one
UPDATE jobs
SET lease_expires_at = @lease_expires_at::timestamptz
WHERE id = @id
  AND status = 'running'
  AND attempt_count = @attempt
  AND lease_owner = @worker_id::text
  AND target_server_id = @server_id::uuid
RETURNING lease_expires_at;

-- name: IsAgentAttemptActive :one
SELECT EXISTS (
    SELECT 1 FROM jobs
    WHERE id = @id
      AND status = 'running'
      AND attempt_count = @attempt
      AND lease_owner = @worker_id::text
      AND target_server_id = @server_id::uuid
);

-- name: AppendJobLogs :execrows
INSERT INTO job_log_chunks (job_id, attempt_number, sequence_number, stream, content)
SELECT @job_id, @attempt_number, unnest(@sequence_numbers::bigint[]), unnest(@streams::text[]), unnest(@contents::text[])
ON CONFLICT (job_id, attempt_number, sequence_number) DO NOTHING;

-- name: CompleteAgentJob :execrows
UPDATE jobs
SET status           = @status::text,
    result           = @result,
    error_code       = @error_code,
    error_message    = @error_message,
    finished_at      = @now::timestamptz,
    lease_expires_at = NULL
WHERE id = @id
  AND status = 'running'
  AND attempt_count = @attempt
  AND lease_owner = @worker_id::text
  AND target_server_id = @server_id::uuid;

-- name: FinishJobAttempt :exec
UPDATE job_attempts
SET status        = @status::text,
    finished_at   = @now::timestamptz,
    error_code    = @error_code,
    error_summary = @error_summary
WHERE job_id = @job_id AND attempt_number = @attempt_number;

-- name: GetAgentAttemptOutcome :one
SELECT status FROM jobs
WHERE id = @id
  AND attempt_count = @attempt
  AND lease_owner = @worker_id::text
  AND target_server_id = @server_id::uuid;

-- name: GetJob :one
SELECT * FROM jobs WHERE id = $1;

-- name: ListJobLogs :many
SELECT attempt_number, sequence_number, stream, content, created_at
FROM job_log_chunks
WHERE job_id = @job_id
  AND (attempt_number, sequence_number) > (@after_attempt::integer, @after_sequence::bigint)
ORDER BY attempt_number, sequence_number
LIMIT @page_limit;
