-- name: CreateRegistryConnection :one
INSERT INTO registry_connections (
    id, project_id, name, host, username, password_ciphertext, password_key_id, created_by
) VALUES (
    @id, @project_id, @name, @host, @username, @password_ciphertext, @password_key_id, @created_by
)
RETURNING *;

-- name: ListRegistryConnections :many
SELECT * FROM registry_connections
WHERE project_id = $1
ORDER BY name, id;

-- name: GetRegistryConnection :one
SELECT * FROM registry_connections WHERE id = $1;

-- name: DeleteRegistryConnection :exec
DELETE FROM registry_connections WHERE id = $1;

-- name: GetRegistryForHost :one
-- The project's registry that hosts an image, preferring the application's own.
SELECT * FROM registry_connections
WHERE project_id = @project_id AND host = @host
ORDER BY COALESCE(id = sqlc.narg(preferred_id)::uuid, false) DESC, created_at, id
LIMIT 1;
