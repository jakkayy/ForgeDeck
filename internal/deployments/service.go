// Package deployments starts deployments of built images into environments
// and reports their state.
package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

var (
	errEnvironmentNotFound = apperr.NotFound("Environment not found.")
	errDeploymentNotFound  = apperr.NotFound("Deployment not found.")
	errBuildNotUsable      = validate.Errors{"build_job_id": "Choose a succeeded build of this application."}

	imageRef = regexp.MustCompile(`^[^\s@]+@sha256:[0-9a-f]{64}$`)
)

// Deployment is the API view of a deployment.
type Deployment struct {
	ID               uuid.UUID  `json:"id"`
	EnvironmentID    uuid.UUID  `json:"environment_id"`
	JobID            *uuid.UUID `json:"job_id"`
	SourceBuildJobID *uuid.UUID `json:"source_build_job_id"`
	SourceReleaseID  *uuid.UUID `json:"source_release_id"`
	CommitSHA        string     `json:"commit_sha"`
	ImageRef         string     `json:"image_ref"`
	Status           string     `json:"status"`
	Stage            *string    `json:"stage"`
	Error            *Error     `json:"error"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	// ReleaseID is set once the deployment became healthy.
	ReleaseID *uuid.UUID `json:"release_id"`
}

// Error says why a deployment failed.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Service implements deployment operations.
type Service struct {
	pool    *pgxpool.Pool
	queries *store.Queries
	authz   *authz.Authorizer
	jobs    *jobs.Service
}

// NewService returns a deployment Service.
func NewService(pool *pgxpool.Pool, authorizer *authz.Authorizer, jobService *jobs.Service) *Service {
	return &Service{pool: pool, queries: store.New(pool), authz: authorizer, jobs: jobService}
}

// Deploy queues a deployment of a succeeded build's image into an
// environment, using the environment's current configuration.
func (s *Service) Deploy(ctx context.Context, userID, environmentID, buildJobID uuid.UUID) (Deployment, error) {
	env, app, err := s.authorizeEnvironment(ctx, userID, environmentID, authz.Deploy)
	if err != nil {
		return Deployment{}, err
	}
	build, err := s.queries.GetJob(ctx, buildJobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, errBuildNotUsable
	}
	if err != nil {
		return Deployment{}, fmt.Errorf("get build job: %w", err)
	}
	result, ok := buildResult(build, app.ID)
	if !ok {
		return Deployment{}, errBuildNotUsable
	}
	config, err := decodeConfig(env.Config)
	if err != nil {
		return Deployment{}, err
	}

	return s.start(ctx, &userID, env, app, source{
		BuildJobID: &build.ID, CommitSHA: result.CommitSHA, ImageRef: result.ImageRef, Config: config,
	})
}

// source is what a deployment puts into an environment.
type source struct {
	BuildJobID *uuid.UUID
	ReleaseID  *uuid.UUID
	CommitSHA  string
	ImageRef   string
	Config     servicecfg.Config
}

// start records the deployment and queues its job in one transaction, so a
// deployment never exists without its job or the reverse.
func (s *Service) start(ctx context.Context, userID *uuid.UUID, env store.Environment, app store.Application, src source) (Deployment, error) {
	var created store.Deployment
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		created, err = s.startIn(ctx, tx, userID, env, app, src)
		return err
	})
	if err != nil {
		return Deployment{}, fmt.Errorf("start deployment: %w", err)
	}
	return toDeployment(created, liveJob{}), nil
}

// startIn does the work of start inside the caller's transaction.
func (s *Service) startIn(ctx context.Context, tx pgx.Tx, userID *uuid.UUID, env store.Environment, app store.Application, src source) (store.Deployment, error) {
	config, err := servicecfg.Normalize(src.Config)
	if err != nil {
		return store.Deployment{}, err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return store.Deployment{}, fmt.Errorf("encode config: %w", err)
	}

	deploymentID := uuid.New()
	job, err := s.jobs.EnqueueIn(ctx, tx, jobs.NewJob{
		ProjectID:      app.ProjectID,
		ApplicationID:  &app.ID,
		Type:           agentproto.JobDeployImage,
		WorkerKind:     jobs.WorkerAgent,
		TargetServerID: &env.ServerID,
		CreatedBy:      userID,
		Payload: agentproto.DeployImagePayload{
			DeploymentID: deploymentID, EnvironmentID: env.ID, ApplicationID: app.ID,
			CommitSHA: src.CommitSHA, ImageRef: src.ImageRef, Config: config,
		},
	})
	if err != nil {
		return store.Deployment{}, err
	}
	return s.queries.WithTx(tx).CreateDeployment(ctx, store.CreateDeploymentParams{
		ID: deploymentID, EnvironmentID: env.ID, JobID: &job.ID,
		SourceBuildJobID: src.BuildJobID, SourceReleaseID: src.ReleaseID,
		CommitSha: src.CommitSHA, ImageRef: src.ImageRef, Config: encoded, CreatedBy: userID,
	})
}

// Get returns a deployment the user may view.
func (s *Service) Get(ctx context.Context, userID, deploymentID uuid.UUID) (Deployment, error) {
	row, err := s.queries.GetDeployment(ctx, deploymentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, errDeploymentNotFound
	}
	if err != nil {
		return Deployment{}, fmt.Errorf("get deployment: %w", err)
	}
	if _, _, err := s.authorizeEnvironment(ctx, userID, row.Deployment.EnvironmentID, authz.ViewProject); err != nil {
		if errors.Is(err, errEnvironmentNotFound) {
			return Deployment{}, errDeploymentNotFound
		}
		return Deployment{}, err
	}
	return s.describe(ctx, row.Deployment, liveJob{
		Status: row.JobStatus, ErrorCode: row.JobErrorCode, ErrorMessage: row.JobErrorMessage,
		StartedAt: row.JobStartedAt, FinishedAt: row.JobFinishedAt,
	}), nil
}

// describe builds the API view of a deployment, adding what is not stored on
// the row: the live stage and the release it produced.
func (s *Service) describe(ctx context.Context, row store.Deployment, job liveJob) Deployment {
	d := toDeployment(row, job)
	d.Stage = s.stageOf(ctx, d)
	if releaseID, err := s.queries.GetReleaseForDeployment(ctx, d.ID); err == nil {
		d.ReleaseID = &releaseID
	}
	return d
}

// List returns an environment's deployments, newest first.
func (s *Service) List(ctx context.Context, userID, environmentID uuid.UUID, limit, offset int32) ([]Deployment, error) {
	if _, _, err := s.authorizeEnvironment(ctx, userID, environmentID, authz.ViewProject); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListDeployments(ctx, store.ListDeploymentsParams{
		EnvironmentID: environmentID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	out := make([]Deployment, len(rows))
	for i, row := range rows {
		out[i] = s.describe(ctx, row.Deployment, liveJob{
			Status: row.JobStatus, ErrorCode: row.JobErrorCode, ErrorMessage: row.JobErrorMessage,
			StartedAt: row.JobStartedAt, FinishedAt: row.JobFinishedAt,
		})
	}
	return out, nil
}

func (s *Service) authorizeEnvironment(ctx context.Context, userID, environmentID uuid.UUID, p authz.Permission) (store.Environment, store.Application, error) {
	env, err := s.queries.GetEnvironment(ctx, environmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Environment{}, store.Application{}, errEnvironmentNotFound
	}
	if err != nil {
		return store.Environment{}, store.Application{}, fmt.Errorf("get environment: %w", err)
	}
	app, err := s.queries.GetApplication(ctx, env.ApplicationID)
	if err != nil {
		return store.Environment{}, store.Application{}, fmt.Errorf("get application: %w", err)
	}
	if _, err := s.authz.Require(ctx, userID, app.ProjectID, p); err != nil {
		if errors.Is(err, authz.ErrProjectNotFound) {
			return store.Environment{}, store.Application{}, errEnvironmentNotFound
		}
		return store.Environment{}, store.Application{}, err
	}
	return env, app, nil
}

// buildResult returns the image a succeeded build of applicationID pushed.
func buildResult(job store.Job, applicationID uuid.UUID) (agentproto.BuildResult, bool) {
	if job.Type != agentproto.JobBuild || job.Status != string(jobs.StatusSucceeded) ||
		job.ApplicationID == nil || *job.ApplicationID != applicationID {
		return agentproto.BuildResult{}, false
	}
	var result agentproto.BuildResult
	if err := json.Unmarshal(job.Result, &result); err != nil || !imageRef.MatchString(result.ImageRef) || result.CommitSHA == "" {
		return agentproto.BuildResult{}, false
	}
	return result, true
}

func unmarshalPayload(job store.Job, into any) error {
	return json.Unmarshal(job.Payload, into)
}

func decodeConfig(raw json.RawMessage) (servicecfg.Config, error) {
	var config servicecfg.Config
	if err := json.Unmarshal(raw, &config); err != nil {
		return servicecfg.Config{}, fmt.Errorf("decode environment config: %w", err)
	}
	return config, nil
}

// liveJob is the state of a deployment's job, when it still exists.
type liveJob struct {
	Status       *string
	ErrorCode    *string
	ErrorMessage *string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// toDeployment prefers the job's live state; the deployment's own columns
// answer once the job is gone.
func toDeployment(row store.Deployment, job liveJob) Deployment {
	d := Deployment{
		ID: row.ID, EnvironmentID: row.EnvironmentID, JobID: row.JobID,
		SourceBuildJobID: row.SourceBuildJobID, SourceReleaseID: row.SourceReleaseID,
		CommitSHA: row.CommitSha, ImageRef: row.ImageRef, Status: row.Status, Stage: row.Stage,
		CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
	}
	code, message := row.ErrorCode, row.ErrorMessage
	if job.Status != nil {
		d.Status, d.StartedAt, d.FinishedAt = *job.Status, job.StartedAt, job.FinishedAt
		code, message = job.ErrorCode, job.ErrorMessage
	}
	if code != nil {
		d.Error = &Error{Code: *code}
		if message != nil {
			d.Error.Message = *message
		}
	}
	return d
}
