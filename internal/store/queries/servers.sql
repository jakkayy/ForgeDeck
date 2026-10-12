-- name: CreateServer :one
INSERT INTO servers (project_id, name, agent_version)
VALUES ($1, $2, $3)
RETURNING *;
