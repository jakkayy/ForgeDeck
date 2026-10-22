// Package environments manages the deployment targets of an application
package environments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

var (
	errEnvironmentNotFound = apperr.NotFound("Environment not found.")
	errApplicationNotFound = apperr.NotFound("Application not found.")
	errServerNotFound      = validate.Errors{"server_id": "Choose a server from this project."}
	errNameTaken           = validate.Errors{"name": "Another environment of this application already uses this name."}
	errPortTaken           = validate.Errors{"published_port": "Another environment already publishes this port on that server."}

	validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

const (
	nameConstraint = "environments_application_name_key"
	portConstraint = "environments_server_port_key"
)

// Environment is the API view of an environment
type Environment struct {
	ID            uuid.UUID         `json:"id"`
	ApplicationID uuid.UUID         `json:"application_id"`
	Name          string            `json:"name"`
	ServerID      uuid.UUID         `json:"server_id"`
	AutoDeploy    bool              `json:"auto_deploy"`
	Config        servicecfg.Config `json:"config"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// SecretChecker says which secret names a project does not have
type SecretChecker interface {
	Missing(ctx context.Context, projectID uuid.UUID, names []string) ([]string, error)
}

// Service implements environment operations
type Service struct {
	queries *store.Queries
	authz   *authz.Authorizer
	secrets SecretChecker
}

// NewService returns an environment Service
func NewService(queries *store.Queries, authorizer *authz.Authorizer, secretChecker SecretChecker) *Service {
	return &Service{queries: queries, authz: authorizer, secrets: secretChecker}
}

// checkSecrets requires every secret an environment lists to exist in the project
func (s *Service) checkSecrets(ctx context.Context, projectID uuid.UUID, config servicecfg.Config) error {
	missing, err := s.secrets.Missing(ctx, projectID, config.Secrets)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return validate.Errors{"secrets": "Unknown secret: " + strings.Join(missing, ", ") + ". Create it in the project's Secrets first."}
	}
	return nil
}

// CreateInput describes a new environment
type CreateInput struct {
	Name       string
	ServerID   uuid.UUID
	AutoDeploy bool
	Config     servicecfg.Config
}

// Create adds an environment to an application
func (s *Service) Create(ctx context.Context, userID, applicationID uuid.UUID, in CreateInput) (Environment, error) {
	app, err := s.authorizeApplication(ctx, userID, applicationID, authz.ManageApps)
	if err != nil {
		return Environment{}, err
	}
	errs := validate.Errors{}
	checkName(errs, in.Name)
	config, configErr := servicecfg.Normalize(in.Config)
	mergeErrors(errs, configErr)
	if err := errs.Err(); err != nil {
		return Environment{}, err
	}
	if err := s.checkServer(ctx, app.ProjectID, in.ServerID); err != nil {
		return Environment{}, err
	}
	if err := s.checkSecrets(ctx, app.ProjectID, config); err != nil {
		return Environment{}, err
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		return Environment{}, fmt.Errorf("encode config: %w", err)
	}
	row, err := s.queries.CreateEnvironment(ctx, store.CreateEnvironmentParams{
		ApplicationID: applicationID, Name: in.Name, ServerID: in.ServerID, AutoDeploy: in.AutoDeploy,
		Config: encoded, PublishedPort: publishedPort(config), CreatedBy: &userID,
	})
	if err != nil {
		return Environment{}, translateWriteError(err, "create environment")
	}
	return toEnvironment(row)
}

// Get returns an environment the user may view
func (s *Service) Get(ctx context.Context, userID, environmentID uuid.UUID) (Environment, error) {
	row, err := s.authorize(ctx, userID, environmentID, authz.ViewProject)
	if err != nil {
		return Environment{}, err
	}
	return toEnvironment(row)
}

// List returns an application's environments by name
func (s *Service) List(ctx context.Context, userID, applicationID uuid.UUID) ([]Environment, error) {
	if _, err := s.authorizeApplication(ctx, userID, applicationID, authz.ViewProject); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListEnvironments(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	out := make([]Environment, len(rows))
	for i, row := range rows {
		if out[i], err = toEnvironment(row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// UpdateInput changes the non-nil fields
type UpdateInput struct {
	Name       *string
	ServerID   *uuid.UUID
	AutoDeploy *bool
	Config     *servicecfg.Config
}

// Update changes an environment
func (s *Service) Update(ctx context.Context, userID, environmentID uuid.UUID, in UpdateInput) (Environment, error) {
	current, err := s.authorize(ctx, userID, environmentID, authz.ManageApps)
	if err != nil {
		return Environment{}, err
	}
	params := store.UpdateEnvironmentParams{ID: environmentID, Name: in.Name, ServerID: in.ServerID, AutoDeploy: in.AutoDeploy}
	errs := validate.Errors{}
	if in.Name != nil {
		checkName(errs, *in.Name)
	}
	if in.Config != nil {
		config, configErr := servicecfg.Normalize(*in.Config)
		mergeErrors(errs, configErr)
		if configErr == nil {
			if params.Config, err = json.Marshal(config); err != nil {
				return Environment{}, fmt.Errorf("encode config: %w", err)
			}
			params.SetConfig, params.PublishedPort = true, publishedPort(config)
		}
	}
	if err := errs.Err(); err != nil {
		return Environment{}, err
	}
	if in.ServerID != nil || in.Config != nil {
		app, err := s.queries.GetApplication(ctx, current.ApplicationID)
		if err != nil {
			return Environment{}, fmt.Errorf("get application: %w", err)
		}
		if in.ServerID != nil {
			if err := s.checkServer(ctx, app.ProjectID, *in.ServerID); err != nil {
				return Environment{}, err
			}
		}
		if in.Config != nil {
			normalized, _ := servicecfg.Normalize(*in.Config) // validated above
			if err := s.checkSecrets(ctx, app.ProjectID, normalized); err != nil {
				return Environment{}, err
			}
		}
	}

	row, err := s.queries.UpdateEnvironment(ctx, params)
	if err != nil {
		return Environment{}, translateWriteError(err, "update environment")
	}
	return toEnvironment(row)
}

// Delete removes an environment
func (s *Service) Delete(ctx context.Context, userID, environmentID uuid.UUID) error {
	if _, err := s.authorize(ctx, userID, environmentID, authz.ManageApps); err != nil {
		return err
	}
	if err := s.queries.DeleteEnvironment(ctx, environmentID); err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	return nil
}

// checkServer requires the server to exist in the application's project
func (s *Service) checkServer(ctx context.Context, projectID, serverID uuid.UUID) error {
	row, err := s.queries.GetServer(ctx, serverID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.Server.ProjectID != projectID) {
		return errServerNotFound
	}
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}
	return nil
}

func (s *Service) authorize(ctx context.Context, userID, environmentID uuid.UUID, p authz.Permission) (store.Environment, error) {
	row, err := s.queries.GetEnvironment(ctx, environmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Environment{}, errEnvironmentNotFound
	}
	if err != nil {
		return store.Environment{}, fmt.Errorf("get environment: %w", err)
	}
	app, err := s.queries.GetApplication(ctx, row.ApplicationID)
	if err != nil {
		return store.Environment{}, fmt.Errorf("get application: %w", err)
	}
	if _, err := s.authz.Require(ctx, userID, app.ProjectID, p); err != nil {
		if errors.Is(err, authz.ErrProjectNotFound) {
			return store.Environment{}, errEnvironmentNotFound
		}
		return store.Environment{}, err
	}
	return row, nil
}

func (s *Service) authorizeApplication(ctx context.Context, userID, applicationID uuid.UUID, p authz.Permission) (store.Application, error) {
	app, err := s.queries.GetApplication(ctx, applicationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Application{}, errApplicationNotFound
	}
	if err != nil {
		return store.Application{}, fmt.Errorf("get application: %w", err)
	}
	if _, err := s.authz.Require(ctx, userID, app.ProjectID, p); err != nil {
		if errors.Is(err, authz.ErrProjectNotFound) {
			return store.Application{}, errApplicationNotFound
		}
		return store.Application{}, err
	}
	return app, nil
}

func checkName(errs validate.Errors, name string) {
	if !validName.MatchString(name) {
		errs.Add("name", "Use lowercase letters, digits and dashes, such as production or staging-eu.")
	}
}

// mergeErrors copies validation failures from err into errs
func mergeErrors(errs validate.Errors, err error) {
	var fields validate.Errors
	if errors.As(err, &fields) {
		for field, message := range fields {
			errs.Add(field, message)
		}
	}
}

func publishedPort(c servicecfg.Config) *int32 {
	if c.PublishedPort == nil {
		return nil
	}
	port := int32(*c.PublishedPort) //nolint:gosec // validated to 1024–65535
	return &port
}

func translateWriteError(err error, action string) error {
	switch {
	case database.IsUniqueViolationOf(err, nameConstraint):
		return errNameTaken
	case database.IsUniqueViolationOf(err, portConstraint):
		return errPortTaken
	default:
		return fmt.Errorf("%s: %w", action, err)
	}
}

func toEnvironment(row store.Environment) (Environment, error) {
	var config servicecfg.Config
	if err := json.Unmarshal(row.Config, &config); err != nil {
		return Environment{}, fmt.Errorf("decode environment config: %w", err)
	}
	return Environment{
		ID: row.ID, ApplicationID: row.ApplicationID, Name: row.Name, ServerID: row.ServerID,
		AutoDeploy: row.AutoDeploy, Config: config, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}
