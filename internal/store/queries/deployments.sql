-- name: CreateDeployment :one
INSERT INTO deployments (
    id, environment_id, job_id, source_build_job_id, source_release_id, commit_sha, image_ref, config, created_by
) VALUES (
    @id, @environment_id, @job_id, @source_build_job_id, @source_release_id, @commit_sha, @image_ref, @config, @created_by
)
RETURNING *;

-- name: GetDeployment :one
-- The job is the live source of status while it exists; the deployment's own
-- columns keep the outcome after the job is pruned.
SELECT sqlc.embed(deployments),
       jobs.status        AS job_status,
       jobs.error_code    AS job_error_code,
       jobs.error_message AS job_error_message,
       jobs.started_at    AS job_started_at,
       jobs.finished_at   AS job_finished_at
FROM deployments
LEFT JOIN jobs ON jobs.id = deployments.job_id
WHERE deployments.id = $1;

-- name: ListDeployments :many
SELECT sqlc.embed(deployments),
       jobs.status        AS job_status,
       jobs.error_code    AS job_error_code,
       jobs.error_message AS job_error_message,
       jobs.started_at    AS job_started_at,
       jobs.finished_at   AS job_finished_at
FROM deployments
LEFT JOIN jobs ON jobs.id = deployments.job_id
WHERE deployments.environment_id = @environment_id
ORDER BY deployments.created_at DESC, deployments.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: HasNewerDeployment :one
-- Whether the environment already deploys, or has queued, a build created
-- after the given time. A slower, older build must not replace a newer one.
SELECT EXISTS (
    SELECT 1 FROM deployments
    JOIN jobs ON jobs.id = deployments.source_build_job_id
    WHERE deployments.environment_id = @environment_id
      AND jobs.created_at > @build_created_at::timestamptz
);
