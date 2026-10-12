-- name: CreateServer :one
INSERT INTO servers (project_id, name, agent_version)
VALUES ($1, $2, $3)
RETURNING *;

-- name: RecordHeartbeat :exec
UPDATE servers
SET last_seen_at     = @now::timestamptz,
    agent_version    = @agent_version,
    docker_available = @docker_available,
    docker_version   = @docker_version,
    cpu_percent      = @cpu_percent,
    memory_percent   = @memory_percent,
    disk_percent     = @disk_percent,
    updated_at       = @now
WHERE id = @id;
