// Package applications manages deployable applications and their build settings.
package applications

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

var (
	errApplicationNotFound = apperr.NotFound("Application not found.")
	errNameTaken           = validate.Errors{"name": "Another application in this project already uses this name."}
)

// Application is the API view of an application.
type Application struct {
	ID                 uuid.UUID `json:"id"`
	ProjectID          uuid.UUID `json:"project_id"`
	Name               string    `json:"name"`
	GitConnectionID    uuid.UUID `json:"git_connection_id"`
	RepositoryID       string    `json:"repository_id"`
	RepositoryFullName string    `json:"repository_full_name"`
	Branch             string    `json:"branch"`
	DockerfilePath     string    `json:"dockerfile_path"`
	BuildContext       string    `json:"build_context"`
	TestCommand        *string   `json:"test_command"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Service implements application operations.
type Service struct {
	queries *store.Queries
	authz   *authz.Authorizer
	git     *gitconn.Service
}

// NewService returns an application Service.
func NewService(queries *store.Queries, authorizer *authz.Authorizer, git *gitconn.Service) *Service {
	return &Service{queries: queries, authz: authorizer, git: git}
}

// CreateInput describes a new application. Branch defaults to the
// repository's default branch.
type CreateInput struct {
	Name            string
	GitConnectionID uuid.UUID
	RepositoryID    string
	Branch          string
	DockerfilePath  string
	BuildContext    string
	TestCommand     string
}

// Create stores an application after checking the repository is reachable
// through the project's connection.
func (s *Service) Create(ctx context.Context, userID, projectID uuid.UUID, in CreateInput) (Application, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ManageApps); err != nil {
		return Application{}, err
	}
	in = withDefaults(in)
	errs := validate.Errors{}
	checkName(errs, in.Name)
	checkRepoPath(errs, "dockerfile_path", in.DockerfilePath)
	checkRepoPath(errs, "build_context", in.BuildContext)
	checkTestCommand(errs, in.TestCommand)
	if in.Branch != "" {
		checkBranch(errs, in.Branch)
	}
	if err := errs.Err(); err != nil {
		return Application{}, err
	}

	repo, err := s.git.FindRepository(ctx, userID, projectID, in.GitConnectionID, in.RepositoryID)
	if err != nil {
		return Application{}, err
	}
	if in.Branch == "" {
		in.Branch = repo.DefaultBranch
	}

	row, err := s.queries.CreateApplication(ctx, store.CreateApplicationParams{
		ProjectID:          projectID,
		Name:               in.Name,
		GitConnectionID:    in.GitConnectionID,
		RepositoryID:       repo.ID,
		RepositoryFullName: repo.FullName,
		Branch:             in.Branch,
		DockerfilePath:     path.Clean(in.DockerfilePath),
		BuildContext:       path.Clean(in.BuildContext),
		TestCommand:        optional(in.TestCommand),
		CreatedBy:          &userID,
	})
	if database.IsUniqueViolation(err) {
		return Application{}, errNameTaken
	}
	if err != nil {
		return Application{}, fmt.Errorf("create application: %w", err)
	}
	return toApplication(row), nil
}

// Get returns an application the user may view.
func (s *Service) Get(ctx context.Context, userID, applicationID uuid.UUID) (Application, error) {
	row, err := s.authorize(ctx, userID, applicationID, authz.ViewProject)
	if err != nil {
		return Application{}, err
	}
	return toApplication(row), nil
}

// List returns a project's applications by name.
func (s *Service) List(ctx context.Context, userID, projectID uuid.UUID, limit, offset int32) ([]Application, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ViewProject); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListApplications(ctx, store.ListApplicationsParams{ProjectID: projectID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	apps := make([]Application, len(rows))
	for i, row := range rows {
		apps[i] = toApplication(row)
	}
	return apps, nil
}

// UpdateInput changes the non-nil fields. TestCommand set to "" clears it.
type UpdateInput struct {
	Name           *string
	Branch         *string
	DockerfilePath *string
	BuildContext   *string
	TestCommand    *string
}

// Update changes build settings. The repository is fixed; create a new
// application to build from another one.
func (s *Service) Update(ctx context.Context, userID, applicationID uuid.UUID, in UpdateInput) (Application, error) {
	if _, err := s.authorize(ctx, userID, applicationID, authz.ManageApps); err != nil {
		return Application{}, err
	}
	params := store.UpdateApplicationParams{ID: applicationID}
	errs := validate.Errors{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		checkName(errs, name)
		params.Name = &name
	}
	if in.Branch != nil {
		branch := strings.TrimSpace(*in.Branch)
		checkBranch(errs, branch)
		params.Branch = &branch
	}
	if in.DockerfilePath != nil {
		checkRepoPath(errs, "dockerfile_path", *in.DockerfilePath)
		cleaned := path.Clean(*in.DockerfilePath)
		params.DockerfilePath = &cleaned
	}
	if in.BuildContext != nil {
		checkRepoPath(errs, "build_context", *in.BuildContext)
		cleaned := path.Clean(*in.BuildContext)
		params.BuildContext = &cleaned
	}
	if in.TestCommand != nil {
		checkTestCommand(errs, *in.TestCommand)
		params.SetTestCommand = true
		params.TestCommand = optional(strings.TrimSpace(*in.TestCommand))
	}
	if err := errs.Err(); err != nil {
		return Application{}, err
	}

	row, err := s.queries.UpdateApplication(ctx, params)
	if database.IsUniqueViolation(err) {
		return Application{}, errNameTaken
	}
	if err != nil {
		return Application{}, fmt.Errorf("update application: %w", err)
	}
	return toApplication(row), nil
}

// Delete removes an application.
func (s *Service) Delete(ctx context.Context, userID, applicationID uuid.UUID) error {
	if _, err := s.authorize(ctx, userID, applicationID, authz.ManageApps); err != nil {
		return err
	}
	if err := s.queries.DeleteApplication(ctx, applicationID); err != nil {
		return fmt.Errorf("delete application: %w", err)
	}
	return nil
}

func (s *Service) authorize(ctx context.Context, userID, applicationID uuid.UUID, p authz.Permission) (store.Application, error) {
	row, err := s.queries.GetApplication(ctx, applicationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Application{}, errApplicationNotFound
	}
	if err != nil {
		return store.Application{}, fmt.Errorf("get application: %w", err)
	}
	if _, err := s.authz.Require(ctx, userID, row.ProjectID, p); err != nil {
		if errors.Is(err, authz.ErrProjectNotFound) {
			return store.Application{}, errApplicationNotFound
		}
		return store.Application{}, err
	}
	return row, nil
}

func withDefaults(in CreateInput) CreateInput {
	in.Name = strings.TrimSpace(in.Name)
	in.Branch = strings.TrimSpace(in.Branch)
	in.TestCommand = strings.TrimSpace(in.TestCommand)
	if in.DockerfilePath = strings.TrimSpace(in.DockerfilePath); in.DockerfilePath == "" {
		in.DockerfilePath = "Dockerfile"
	}
	if in.BuildContext = strings.TrimSpace(in.BuildContext); in.BuildContext == "" {
		in.BuildContext = "."
	}
	return in
}

func toApplication(row store.Application) Application {
	return Application{
		ID: row.ID, ProjectID: row.ProjectID, Name: row.Name, GitConnectionID: row.GitConnectionID,
		RepositoryID: row.RepositoryID, RepositoryFullName: row.RepositoryFullName, Branch: row.Branch,
		DockerfilePath: row.DockerfilePath, BuildContext: row.BuildContext, TestCommand: row.TestCommand,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
