-- name: GetLatestRelease :one
SELECT * FROM releases
WHERE environment_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CreateRelease :one
-- A deployment becomes a release once; recording it again changes nothing.
INSERT INTO releases (application_id, environment_id, deployment_id, commit_sha, image_ref, config)
VALUES (@application_id, @environment_id, @deployment_id, @commit_sha, @image_ref, @config)
ON CONFLICT (deployment_id) DO NOTHING
RETURNING *;

-- name: GetRelease :one
SELECT * FROM releases WHERE id = $1;

-- name: ListReleases :many
SELECT releases.*,
       releases.id = (
           SELECT latest.id FROM releases AS latest
           WHERE latest.environment_id = releases.environment_id
           ORDER BY latest.created_at DESC, latest.id DESC LIMIT 1
       ) AS is_current
FROM releases
WHERE releases.environment_id = @environment_id
ORDER BY releases.created_at DESC, releases.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetReleaseForDeployment :one
SELECT id FROM releases WHERE deployment_id = $1;
