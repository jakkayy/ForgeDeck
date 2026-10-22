-- name: MarkExpiredJobLogs :many
-- Marks finished jobs older than the cutoff in one step so each is handled once; only logs go.
WITH due AS (
    SELECT id FROM jobs
    WHERE finished_at < @cutoff::timestamptz
      AND status IN ('succeeded', 'failed', 'cancelled')
      AND logs_expired_at IS NULL
    ORDER BY finished_at
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
)
UPDATE jobs SET logs_expired_at = @now::timestamptz
FROM due
WHERE jobs.id = due.id
RETURNING jobs.id;

-- name: DeleteJobLogLines :execrows
DELETE FROM job_log_chunks WHERE job_id = ANY(@job_ids::uuid[]);

-- name: DeleteOldJobs :execrows
-- Finished jobs, with their attempts and logs, once they are old enough.
DELETE FROM jobs
WHERE id IN (
    SELECT id FROM jobs
    WHERE finished_at < @cutoff::timestamptz
      AND status IN ('succeeded', 'failed', 'cancelled')
    ORDER BY finished_at
    LIMIT @batch
);

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE id IN (SELECT id FROM sessions WHERE expires_at < @cutoff::timestamptz LIMIT @batch);

-- name: DeleteOldEnrollmentTokens :execrows
-- Tokens that were used or have expired, long ago.
DELETE FROM enrollment_tokens
WHERE id IN (
    SELECT id FROM enrollment_tokens
    WHERE COALESCE(used_at, expires_at) < @cutoff::timestamptz
    LIMIT @batch
);

-- name: DeleteOldWebhookEvents :execrows
DELETE FROM webhook_events
WHERE id IN (SELECT id FROM webhook_events WHERE received_at < @cutoff::timestamptz LIMIT @batch);

-- name: DeleteOldAuditLogs :execrows
DELETE FROM audit_logs
WHERE id IN (SELECT id FROM audit_logs WHERE created_at < @cutoff::timestamptz LIMIT @batch);
