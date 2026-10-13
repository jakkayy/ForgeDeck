-- name: CountServersByConnectivity :one
-- Mirrors servers.ConnectivityStatus: the caller passes the heartbeat
-- thresholds, so the windows are defined in one place.
SELECT
    count(*)                                                        AS total,
    count(*) FILTER (WHERE servers.last_seen_at IS NULL)            AS pending,
    count(*) FILTER (WHERE servers.last_seen_at >= @online_after::timestamptz) AS online,
    count(*) FILTER (WHERE servers.last_seen_at < @online_after::timestamptz
                       AND servers.last_seen_at >= @unreachable_after::timestamptz) AS unreachable,
    count(*) FILTER (WHERE servers.last_seen_at < @unreachable_after::timestamptz) AS offline
FROM servers
JOIN project_members ON project_members.project_id = servers.project_id
WHERE project_members.user_id = @user_id;

-- name: CountEnvironmentsByLastDeployment :one
-- Groups the user's environments by how their newest deployment ended. The
-- job is the live source of status while it exists (see ListDeployments).
WITH latest AS (
    SELECT DISTINCT ON (environments.id)
           COALESCE(jobs.status, deployments.status) AS status
    FROM environments
    JOIN applications ON applications.id = environments.application_id
    JOIN project_members ON project_members.project_id = applications.project_id
                        AND project_members.user_id = @user_id
    LEFT JOIN deployments ON deployments.environment_id = environments.id
    LEFT JOIN jobs ON jobs.id = deployments.job_id
    ORDER BY environments.id, deployments.created_at DESC NULLS LAST, deployments.id DESC
)
SELECT
    count(*)                                                    AS total,
    count(*) FILTER (WHERE status = 'succeeded')                AS succeeded,
    count(*) FILTER (WHERE status = 'failed')                   AS failed,
    count(*) FILTER (WHERE status IN ('queued', 'running'))     AS in_progress,
    count(*) FILTER (WHERE status IS NULL OR status = 'cancelled') AS not_deployed
FROM latest;

-- name: ListRecentDeploymentsForUser :many
-- Newest deployments across the user's projects. When only_failed is set, only
-- the newest deployment of each environment, and only if it failed: what still
-- needs attention, not history.
SELECT deployments.id,
       deployments.environment_id,
       environments.name                                 AS environment_name,
       applications.id                                   AS application_id,
       applications.name                                 AS application_name,
       projects.name                                     AS project_name,
       deployments.commit_sha,
       COALESCE(jobs.status, deployments.status)         AS status,
       COALESCE(jobs.error_code, deployments.error_code) AS error_code,
       deployments.created_at,
       COALESCE(jobs.finished_at, deployments.finished_at) AS finished_at
FROM deployments
JOIN environments ON environments.id = deployments.environment_id
JOIN applications ON applications.id = environments.application_id
JOIN projects ON projects.id = applications.project_id
JOIN project_members ON project_members.project_id = applications.project_id
                    AND project_members.user_id = @user_id
LEFT JOIN jobs ON jobs.id = deployments.job_id
WHERE (NOT @only_failed::boolean OR (
        COALESCE(jobs.status, deployments.status) = 'failed'
        AND deployments.id = (SELECT d.id FROM deployments d
                              WHERE d.environment_id = deployments.environment_id
                              ORDER BY d.created_at DESC, d.id DESC LIMIT 1)))
ORDER BY deployments.created_at DESC, deployments.id DESC
LIMIT @page_limit;
