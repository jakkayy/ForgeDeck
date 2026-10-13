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

-- name: GetServer :one
SELECT sqlc.embed(servers), projects.name AS project_name
FROM servers
JOIN projects ON projects.id = servers.project_id
WHERE servers.id = $1;

-- name: ListServersForUser :many
SELECT sqlc.embed(servers), projects.name AS project_name
FROM servers
JOIN projects ON projects.id = servers.project_id
JOIN project_members ON project_members.project_id = servers.project_id
WHERE project_members.user_id = @user_id
  AND (sqlc.narg(search)::text IS NULL OR servers.name ILIKE '%' || sqlc.narg(search)::text || '%')
  -- Connectivity is derived from the heartbeat age (servers.ConnectivityStatus);
  -- the caller passes the cutoffs so the windows live in one place.
  AND (sqlc.narg(status)::text IS NULL
       OR (sqlc.narg(status)::text = 'pending' AND servers.last_seen_at IS NULL)
       OR (sqlc.narg(status)::text = 'online' AND servers.last_seen_at >= @online_after::timestamptz)
       OR (sqlc.narg(status)::text = 'unreachable' AND servers.last_seen_at < @online_after::timestamptz
                                                   AND servers.last_seen_at >= @unreachable_after::timestamptz)
       OR (sqlc.narg(status)::text = 'offline' AND servers.last_seen_at < @unreachable_after::timestamptz))
ORDER BY projects.name, servers.name, servers.id
LIMIT @page_limit OFFSET @page_offset;

-- name: ListProjectServers :many
SELECT sqlc.embed(servers), projects.name AS project_name
FROM servers
JOIN projects ON projects.id = servers.project_id
WHERE servers.project_id = @project_id
ORDER BY servers.name, servers.id
LIMIT @page_limit OFFSET @page_offset;

-- name: DeleteServer :exec
DELETE FROM servers WHERE id = $1;
