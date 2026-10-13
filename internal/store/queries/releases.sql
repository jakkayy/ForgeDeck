-- name: GetLatestRelease :one
SELECT * FROM releases
WHERE environment_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1;
