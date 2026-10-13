-- name: CreateEnvironment :one
INSERT INTO environments (application_id, name, server_id, auto_deploy, config, published_port, created_by)
VALUES (@application_id, @name, @server_id, @auto_deploy, @config, @published_port, @created_by)
RETURNING *;

-- name: GetEnvironment :one
SELECT * FROM environments WHERE id = $1;

-- name: ListEnvironments :many
SELECT * FROM environments
WHERE application_id = $1
ORDER BY name, id;

-- name: UpdateEnvironment :one
UPDATE environments
SET name           = COALESCE(sqlc.narg(name), name),
    server_id      = COALESCE(sqlc.narg(server_id), server_id),
    auto_deploy    = COALESCE(sqlc.narg(auto_deploy), auto_deploy),
    config         = COALESCE(sqlc.narg(config), config),
    published_port = CASE WHEN @set_config::boolean THEN sqlc.narg(published_port) ELSE published_port END,
    updated_at     = now()
WHERE id = @id
RETURNING *;

-- name: DeleteEnvironment :exec
DELETE FROM environments WHERE id = $1;
