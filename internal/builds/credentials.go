// Package builds connects build jobs to the services they depend on
package builds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/registries"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// Error is a build that cannot start, with a code and message for users
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

var (
	errApplicationGone = &Error{Code: "application_not_found", Message: "The application was deleted before the build started."}
	errNoImage         = &Error{Code: "no_image_destination", Message: "Choose a registry and image for this application, then build again."}
	errNoRepoAccess    = &Error{Code: "build_credentials_unavailable", Message: "ForgeDeck could not get access to the repository for this build. Check the application's Git connection."}
	errNoRegistry      = &Error{Code: "build_credentials_unavailable", Message: "ForgeDeck could not read the registry credentials for this build. Check the application's registry."}
)

// Credentials mints build credentials at claim time
type Credentials struct {
	queries    *store.Queries
	git        *gitconn.Service
	registries *registries.Service
}

// NewCredentials returns a Credentials minter
func NewCredentials(queries *store.Queries, git *gitconn.Service, registryService *registries.Service) *Credentials {
	return &Credentials{queries: queries, git: git, registries: registryService}
}

// Mint returns the credentials for one attempt of the build described by payload
func (c *Credentials) Mint(ctx context.Context, payload json.RawMessage) (*agentproto.BuildCredentials, error) {
	var build agentproto.BuildPayload
	if err := json.Unmarshal(payload, &build); err != nil {
		return nil, fmt.Errorf("decode build payload: %w", err)
	}
	app, err := c.queries.GetApplication(ctx, build.ApplicationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errApplicationGone
	}
	if err != nil {
		return nil, fmt.Errorf("get application: %w", err)
	}
	if app.RegistryConnectionID == nil || app.ImageRepository == nil {
		return nil, errNoImage
	}

	git, err := c.git.CloneCredentials(ctx, build.GitConnectionID, build.RepositoryID, build.RepositoryFullName)
	if err != nil {
		return nil, fmt.Errorf("%w (%w)", errNoRepoAccess, err)
	}
	registry, err := c.registries.Credentials(ctx, *app.RegistryConnectionID)
	if err != nil {
		return nil, fmt.Errorf("%w (%w)", errNoRegistry, err)
	}
	return &agentproto.BuildCredentials{
		Git:      git,
		Registry: agentproto.RegistryCredentials{Host: registry.Host, Username: registry.Username, Password: registry.Password},
		Image:    registry.Host + "/" + *app.ImageRepository,
	}, nil
}
