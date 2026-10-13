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

-- name: RecoverExpiredLeases :many
-- Requeues running jobs whose worker stopped renewing the lease, or fails
-- them when no attempts are left, and marks the abandoned attempt as lost.
WITH expired AS (
    SELECT stale.id FROM jobs AS stale
    WHERE stale.status = 'running' AND stale.lease_expires_at < @now::timestamptz
    FOR UPDATE SKIP LOCKED
), recovered AS (
    UPDATE jobs
    SET status           = CASE WHEN jobs.attempt_count < jobs.max_attempts THEN 'queued' ELSE 'failed' END,
        lease_expires_at = NULL,
        error_code       = CASE WHEN jobs.attempt_count < jobs.max_attempts THEN NULL ELSE 'lease_expired' END,
        error_message    = CASE WHEN jobs.attempt_count < jobs.max_attempts THEN NULL
                                ELSE 'The worker stopped responding before the job finished.' END,
        finished_at      = CASE WHEN jobs.attempt_count < jobs.max_attempts THEN NULL ELSE @now::timestamptz END
    FROM expired
    WHERE jobs.id = expired.id
    RETURNING jobs.id, jobs.status, jobs.attempt_count, jobs.worker_kind, jobs.target_server_id
), lost AS (
    UPDATE job_attempts
    SET status = 'lost', finished_at = @now::timestamptz
    FROM recovered
    WHERE job_attempts.job_id = recovered.id AND job_attempts.attempt_number = recovered.attempt_count
)
SELECT id, status, worker_kind, target_server_id FROM recovered;

-- name: CancelJob :one
-- Cancels a queued or running job and returns the status it had before.
WITH target AS (
    SELECT current.id, current.status, current.attempt_count FROM jobs AS current
    WHERE current.id = @id
    FOR UPDATE
), cancelled AS (
    UPDATE jobs
    SET status = 'cancelled', finished_at = @now::timestamptz, lease_expires_at = NULL
    FROM target
    WHERE jobs.id = target.id AND target.status IN ('queued', 'running')
), attempt AS (
    UPDATE job_attempts
    SET status = 'cancelled', finished_at = @now::timestamptz
    FROM target
    WHERE job_attempts.job_id = target.id
      AND job_attempts.attempt_number = target.attempt_count
      AND target.status = 'running'
)
SELECT status FROM target;

-- name: ListJobsForUser :many
-- Newest first, only in projects the user belongs to. The cursor is the
-- (created_at, id) of the last job on the previous page.
SELECT jobs.* FROM jobs
JOIN project_members ON project_members.project_id = jobs.project_id AND project_members.user_id = @user_id
WHERE (sqlc.narg(project_id)::uuid IS NULL OR jobs.project_id = sqlc.narg(project_id)::uuid)
  AND (sqlc.narg(application_id)::uuid IS NULL OR jobs.application_id = sqlc.narg(application_id)::uuid)
  AND (sqlc.narg(server_id)::uuid IS NULL OR jobs.target_server_id = sqlc.narg(server_id)::uuid)
  AND (sqlc.narg(status)::text IS NULL OR jobs.status = sqlc.narg(status)::text)
  AND (sqlc.narg(type)::text IS NULL OR jobs.type = sqlc.narg(type)::text)
  AND (sqlc.narg(before_created_at)::timestamptz IS NULL
       OR (jobs.created_at, jobs.id) < (sqlc.narg(before_created_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY jobs.created_at DESC, jobs.id DESC
LIMIT @page_limit;
