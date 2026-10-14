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
-- job is the live source of status while it exists (see ListDeployments). The
-- newest deployment of each environment is one index lookup, not a sort of
-- every deployment there is.
WITH latest AS (
    SELECT COALESCE(newest.job_status, newest.status) AS status
    FROM environments
    JOIN applications ON applications.id = environments.application_id
    JOIN project_members ON project_members.project_id = applications.project_id
                        AND project_members.user_id = @user_id
    LEFT JOIN LATERAL (
        SELECT d.status, j.status AS job_status
        FROM deployments d
        LEFT JOIN jobs j ON j.id = d.job_id
        WHERE d.environment_id = environments.id
        ORDER BY d.created_at DESC, d.id DESC
        LIMIT 1
    ) newest ON true
)
SELECT
    count(*)                                                    AS total,
    count(*) FILTER (WHERE status = 'succeeded')                AS succeeded,
    count(*) FILTER (WHERE status = 'failed')                   AS failed,
    count(*) FILTER (WHERE status IN ('queued', 'running'))     AS in_progress,
    count(*) FILTER (WHERE status IS NULL OR status = 'cancelled') AS not_deployed
FROM latest;

-- name: ListRecentDeploymentsForUser :many
-- Newest deployments across the user's projects: each environment offers its
-- newest few (an index range), and the newest of those are kept. When
-- only_failed is set, each offers only its single newest deployment and only a
-- failed one counts: what still needs attention, not history.
SELECT d.id,
       environments.id                                   AS environment_id,
       environments.name                                 AS environment_name,
       applications.id                                   AS application_id,
       applications.name                                 AS application_name,
       projects.name                                     AS project_name,
       d.commit_sha,
       d.status,
       d.error_code,
       d.created_at,
       d.finished_at
FROM environments
JOIN applications ON applications.id = environments.application_id
JOIN projects ON projects.id = applications.project_id
JOIN project_members ON project_members.project_id = applications.project_id
                    AND project_members.user_id = @user_id
CROSS JOIN LATERAL (
    SELECT deployments.id,
           deployments.commit_sha,
           COALESCE(jobs.status, deployments.status)         AS status,
           COALESCE(jobs.error_code, deployments.error_code) AS error_code,
           deployments.created_at,
           COALESCE(jobs.finished_at, deployments.finished_at) AS finished_at
    FROM deployments
    LEFT JOIN jobs ON jobs.id = deployments.job_id
    WHERE deployments.environment_id = environments.id
    ORDER BY deployments.created_at DESC, deployments.id DESC
    LIMIT (CASE WHEN @only_failed::boolean THEN 1 ELSE @page_limit::int END)
) d
WHERE NOT @only_failed::boolean OR d.status = 'failed'
ORDER BY d.created_at DESC, d.id DESC
LIMIT @page_limit;
