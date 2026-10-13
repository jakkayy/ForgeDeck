package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/registries"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// MintError is a deployment that cannot start, with a code and message for
// users.
type MintError struct {
	Code    string
	Message string
}

func (e *MintError) Error() string { return e.Code + ": " + e.Message }

var (
	errEnvironmentGone = &MintError{Code: "environment_not_found", Message: "The environment was deleted before the deployment started."}
	errNoRegistryCreds = &MintError{Code: "registry_credentials_unavailable", Message: "ForgeDeck could not read the registry credentials for this image. Check the registry in the project."}
)

// ContextMinter builds the DeployContext of a claimed deploy_image job.
type ContextMinter struct {
	queries    *store.Queries
	registries *registries.Service
}

// NewContextMinter returns a ContextMinter.
func NewContextMinter(queries *store.Queries, registryService *registries.Service) *ContextMinter {
	return &ContextMinter{queries: queries, registries: registryService}
}

// Mint returns the context for one attempt of the deployment in payload.
// Registry credentials are scoped to the registry host of the image: other
// registries of the project are never sent to the server. Errors users can
// act on are *MintError; others are internal.
func (m *ContextMinter) Mint(ctx context.Context, payload json.RawMessage) (*agentproto.DeployContext, error) {
	var p agentproto.DeployImagePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("decode deploy payload: %w", err)
	}
	app, err := m.queries.GetApplication(ctx, p.ApplicationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errEnvironmentGone
	}
	if err != nil {
		return nil, fmt.Errorf("get application: %w", err)
	}
	if _, err := m.queries.GetEnvironment(ctx, p.EnvironmentID); errors.Is(err, pgx.ErrNoRows) {
		return nil, errEnvironmentGone
	} else if err != nil {
		return nil, fmt.Errorf("get environment: %w", err)
	}

	out := &agentproto.DeployContext{}
	host, _, _ := strings.Cut(p.ImageRef, "/")
	registry, err := m.queries.GetRegistryForHost(ctx, store.GetRegistryForHostParams{
		ProjectID: app.ProjectID, Host: host, PreferredID: app.RegistryConnectionID,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("get registry: %w", err)
	default:
		creds, err := m.registries.Credentials(ctx, registry.ID)
		if err != nil {
			return nil, fmt.Errorf("%w (%w)", errNoRegistryCreds, err)
		}
		out.Registry = &agentproto.RegistryCredentials{Host: creds.Host, Username: creds.Username, Password: creds.Password}
	}

	release, err := m.queries.GetLatestRelease(ctx, p.EnvironmentID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("get latest release: %w", err)
	default:
		config, err := decodeConfig(release.Config)
		if err != nil {
			return nil, err
		}
		out.Previous = &agentproto.PreviousRelease{ReleaseID: release.ID, CommitSHA: release.CommitSha, ImageRef: release.ImageRef, Config: config}
	}
	return out, nil
}
