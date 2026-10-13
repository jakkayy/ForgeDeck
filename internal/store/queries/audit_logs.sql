-- name: InsertAuditLog :exec
INSERT INTO audit_logs (project_id, actor_user_id, actor, action, target_type, target_id, target_name, metadata, request_id)
VALUES (@project_id, @actor_user_id, @actor, @action, @target_type, @target_id, @target_name, @metadata, @request_id);

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE project_id = @project_id
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action)::text)
  AND (sqlc.narg(before_created_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(before_created_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;
