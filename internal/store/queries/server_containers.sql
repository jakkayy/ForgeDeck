-- name: LockServerSnapshotTime :one
SELECT containers_observed_at FROM servers WHERE id = $1 FOR UPDATE;

-- name: DeleteServerContainers :exec
DELETE FROM server_containers WHERE server_id = $1;

-- name: InsertServerContainers :copyfrom
INSERT INTO server_containers (server_id, container_id, name, image, state, status_text, labels, observed_at, received_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: SetServerSnapshotTime :exec
UPDATE servers SET containers_observed_at = $2 WHERE id = $1;

-- name: ListServerContainers :many
SELECT * FROM server_containers
WHERE server_id = $1
ORDER BY name, container_id;

-- name: ServerHasContainer :one
SELECT EXISTS (
    SELECT 1 FROM server_containers WHERE server_id = $1 AND container_id = $2
);
