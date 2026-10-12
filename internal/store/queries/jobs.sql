-- name: InsertJob :one
INSERT INTO jobs (project_id, type, worker_kind, target_server_id, payload, max_attempts, idempotency_key, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

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
