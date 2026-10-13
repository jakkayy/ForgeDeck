package deployments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// Release is the API view of a release: a deployment that passed its health
// check, with the configuration it ran with.
type Release struct {
	ID            uuid.UUID         `json:"id"`
	ApplicationID uuid.UUID         `json:"application_id"`
	EnvironmentID uuid.UUID         `json:"environment_id"`
	DeploymentID  uuid.UUID         `json:"deployment_id"`
	CommitSHA     string            `json:"commit_sha"`
	ImageRef      string            `json:"image_ref"`
	Config        servicecfg.Config `json:"config"`
	CreatedAt     time.Time         `json:"created_at"`
	// Current marks the release the environment runs now: its newest one.
	Current bool `json:"current"`
}

// RecordRelease is a jobs.SuccessHook: a deploy_image job succeeds only
// after its health check passed, so the deployment becomes a release in the
// same transaction that records the job's outcome. Recording is idempotent.
func (s *Service) RecordRelease(ctx context.Context, tx pgx.Tx, job store.Job) error {
	if job.Type != agentproto.JobDeployImage {
		return nil
	}
	q := s.queries.WithTx(tx)
	deployment, err := q.GetDeploymentByJob(ctx, &job.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // the deployment (or its environment) was deleted meanwhile
	}
	if err != nil {
		return fmt.Errorf("record release: get deployment: %w", err)
	}
	env, err := q.GetEnvironment(ctx, deployment.EnvironmentID)
	if err != nil {
		return fmt.Errorf("record release: get environment: %w", err)
	}
	if err := q.MarkDeploymentSucceeded(ctx, deployment.ID); err != nil {
		return fmt.Errorf("record release: mark deployment: %w", err)
	}
	_, err = q.CreateRelease(ctx, store.CreateReleaseParams{
		ApplicationID: env.ApplicationID, EnvironmentID: env.ID, DeploymentID: deployment.ID,
		CommitSha: deployment.CommitSha, ImageRef: deployment.ImageRef, Config: deployment.Config,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already recorded
	}
	if err != nil {
		return fmt.Errorf("record release: create: %w", err)
	}
	return nil
}

// ListReleases returns an environment's releases, newest first.
func (s *Service) ListReleases(ctx context.Context, userID, environmentID uuid.UUID, limit, offset int32) ([]Release, error) {
	if _, _, err := s.authorizeEnvironment(ctx, userID, environmentID, authz.ViewProject); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListReleases(ctx, store.ListReleasesParams{EnvironmentID: environmentID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	out := make([]Release, len(rows))
	for i, row := range rows {
		release, err := toRelease(store.Release{
			ID: row.ID, ApplicationID: row.ApplicationID, EnvironmentID: row.EnvironmentID, DeploymentID: row.DeploymentID,
			CommitSha: row.CommitSha, ImageRef: row.ImageRef, Config: row.Config, CreatedAt: row.CreatedAt,
		})
		if err != nil {
			return nil, err
		}
		release.Current = row.IsCurrent
		out[i] = release
	}
	return out, nil
}

func toRelease(row store.Release) (Release, error) {
	config, err := decodeConfig(row.Config)
	if err != nil {
		return Release{}, err
	}
	return Release{
		ID: row.ID, ApplicationID: row.ApplicationID, EnvironmentID: row.EnvironmentID, DeploymentID: row.DeploymentID,
		CommitSHA: row.CommitSha, ImageRef: row.ImageRef, Config: config, CreatedAt: row.CreatedAt,
	}, nil
}

// stageOfError maps a deployment's failure code to the stage that failed.
var stageOfError = map[string]string{
	"registry_login_failed": agentproto.StagePull,
	"pull_failed":           agentproto.StagePull,
	"start_failed":          agentproto.StageStart,
	"health_check_failed":   agentproto.StageHealthCheck,
}

// stageOf says which stage a deployment is in or failed at. A running
// deployment reports the newest stage the Agent logged; a failed one the
// stage its error code names (the log's last stage can be a later recovery
// step). Queued and succeeded deployments have none.
func (s *Service) stageOf(ctx context.Context, d Deployment) *string {
	var stage string
	switch jobs.Status(d.Status) {
	case jobs.StatusRunning:
		if d.JobID == nil {
			return nil
		}
		line, err := s.queries.LatestStageLine(ctx, *d.JobID)
		if err != nil {
			return nil
		}
		stage = agentproto.StageOfLine(line)
	case jobs.StatusFailed:
		if d.Error != nil {
			stage = stageOfError[d.Error.Code]
		}
	}
	if stage == "" {
		return nil
	}
	return &stage
}
