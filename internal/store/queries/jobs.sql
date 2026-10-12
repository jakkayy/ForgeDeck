-- name: InsertJob :one
INSERT INTO jobs (project_id, type, worker_kind, target_server_id, payload, max_attempts, idempotency_key, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: NotifyJobQueued :exec
SELECT pg_notify(@channel::text, @payload::text);
