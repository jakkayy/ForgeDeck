package builds

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

var (
	errApplicationNotFound = apperr.NotFound("Application not found.")
	errImageNotSet         = apperr.Conflict("no_image_destination", "Choose a registry and image for this application before building it.")
)

// NewJob describes the build of one commit of app. Webhook builds add an
// idempotency key so a redelivered push is built once.
func NewJob(app store.Application, branch, commitSHA, commitMessage string) jobs.NewJob {
	return jobs.NewJob{
		ProjectID:     app.ProjectID,
		ApplicationID: &app.ID,
		Type:          agentproto.JobBuild,
		WorkerKind:    jobs.WorkerRunner,
		Payload: agentproto.BuildPayload{
			ApplicationID:      app.ID,
			GitConnectionID:    app.GitConnectionID,
			RepositoryID:       app.RepositoryID,
			RepositoryFullName: app.RepositoryFullName,
			Branch:             branch,
			CommitSHA:          commitSHA,
			CommitMessage:      commitMessage,
			DockerfilePath:     app.DockerfilePath,
			BuildContext:       app.BuildContext,
			TestCommand:        app.TestCommand,
		},
	}
}

// Queued is a build job accepted for running.
type Queued struct {
	JobID     uuid.UUID   `json:"job_id"`
	Status    jobs.Status `json:"status"`
	CommitSHA string      `json:"commit_sha"`
}

// Service starts builds on request.
type Service struct {
	queries *store.Queries
	authz   *authz.Authorizer
	git     *gitconn.Service
	jobs    *jobs.Service
}

// NewService returns a build Service.
func NewService(queries *store.Queries, authorizer *authz.Authorizer, git *gitconn.Service, jobService *jobs.Service) *Service {
	return &Service{queries: queries, authz: authorizer, git: git, jobs: jobService}
}

// Trigger queues a build of commitSHA, or of the head of the application's
// branch when commitSHA is empty. The commit is resolved through the Git
// provider, so only commits that exist in the repository are built. If the
// same commit is already queued or building, that build is returned.
func (s *Service) Trigger(ctx context.Context, userID, applicationID uuid.UUID, commitSHA string) (Queued, error) {
	commitSHA = strings.ToLower(strings.TrimSpace(commitSHA))
	if commitSHA != "" && !validCommit(commitSHA) {
		return Queued{}, validate.Errors{"commit_sha": "Use a full 40-character commit SHA, or leave it empty to build the branch head."}
	}
	app, err := s.authorize(ctx, userID, applicationID)
	if err != nil {
		return Queued{}, err
	}
	if app.RegistryConnectionID == nil {
		return Queued{}, errImageNotSet
	}

	ref := commitSHA
	if ref == "" {
		ref = app.Branch
	}
	commit, err := s.git.ResolveCommit(ctx, app.GitConnectionID, app.RepositoryID, app.RepositoryFullName, ref)
	if err != nil {
		return Queued{}, err
	}

	active, err := s.queries.FindActiveBuild(ctx, store.FindActiveBuildParams{ApplicationID: &app.ID, CommitSha: commit.SHA})
	if err == nil {
		return Queued{JobID: active.ID, Status: jobs.Status(active.Status), CommitSHA: commit.SHA}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Queued{}, fmt.Errorf("find active build: %w", err)
	}

	job := NewJob(app, app.Branch, commit.SHA, firstLine(commit.Message))
	job.CreatedBy = &userID
	stored, err := s.jobs.Enqueue(ctx, job)
	if err != nil {
		return Queued{}, err
	}
	return Queued{JobID: stored.ID, Status: jobs.Status(stored.Status), CommitSHA: commit.SHA}, nil
}

func (s *Service) authorize(ctx context.Context, userID, applicationID uuid.UUID) (store.Application, error) {
	app, err := s.queries.GetApplication(ctx, applicationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Application{}, errApplicationNotFound
	}
	if err != nil {
		return store.Application{}, fmt.Errorf("get application: %w", err)
	}
	if _, err := s.authz.Require(ctx, userID, app.ProjectID, authz.RunBuilds); err != nil {
		if errors.Is(err, authz.ErrProjectNotFound) {
			return store.Application{}, errApplicationNotFound
		}
		return store.Application{}, err
	}
	return app, nil
}

func validCommit(sha string) bool {
	if len(sha) != 40 && len(sha) != 64 {
		return false
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func firstLine(message string) string {
	line, _, _ := strings.Cut(message, "\n")
	return line
}
