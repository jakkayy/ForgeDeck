package deployments

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

var (
	errReleaseNotFound = apperr.NotFound("Release not found.")
	errPortInUse       = apperr.Conflict("port_in_use", "Another environment on this server now publishes the port this release used. Change that environment first.")
)

// Rollback deploys a previous release again: the same image digest and the
// configuration it ran with, never a rebuild of old source. It is an
// ordinary deployment (source: the release) and, once healthy, becomes the
// environment's newest release; history is not rewritten.
//
// The environment's own configuration is left alone, so later deployments
// still use what the user set there.
func (s *Service) Rollback(ctx context.Context, userID, releaseID uuid.UUID) (Deployment, error) {
	release, err := s.queries.GetRelease(ctx, releaseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, errReleaseNotFound
	}
	if err != nil {
		return Deployment{}, fmt.Errorf("get release: %w", err)
	}
	env, app, err := s.authorizeEnvironment(ctx, userID, release.EnvironmentID, authz.Deploy)
	if err != nil {
		if errors.Is(err, errEnvironmentNotFound) {
			return Deployment{}, errReleaseNotFound
		}
		return Deployment{}, err
	}
	config, err := decodeConfig(release.Config)
	if err != nil {
		return Deployment{}, err
	}
	if err := s.checkPortFree(ctx, env, config.PublishedPort); err != nil {
		return Deployment{}, err
	}
	return s.start(ctx, &userID, env, app, source{
		ReleaseID: &release.ID, CommitSHA: release.CommitSha, ImageRef: release.ImageRef, Config: config,
	})
}

// checkPortFree refuses a configuration whose host port another environment
// on the same server publishes now.
func (s *Service) checkPortFree(ctx context.Context, env store.Environment, port *int) error {
	if port == nil {
		return nil
	}
	_, err := s.queries.FindPortConflict(ctx, store.FindPortConflictParams{
		ServerID: env.ServerID, PublishedPort: publishedPort(*port), ExceptID: env.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check port: %w", err)
	}
	return errPortInUse
}

func publishedPort(port int) *int32 {
	p := int32(port) //nolint:gosec // validated to 1024–65535 when the release was created
	return &p
}
