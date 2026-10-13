package gitconn

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
)

var errPublicURLRequired = apperr.Conflict("public_url_required",
	"GitLab needs to reach ForgeDeck to send pushes. Set FORGEDECK_PUBLIC_URL and try again.")

// EnsureWebhook makes the provider send pushes for a repository. GitHub App
// webhooks already cover every repository, so only GitLab needs a hook; its
// ID is returned so it can be removed later.
func (s *Service) EnsureWebhook(ctx context.Context, connectionID uuid.UUID, repositoryID string) (*string, error) {
	row, err := s.queries.GetGitConnection(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("get git connection: %w", err)
	}
	if row.Provider != "gitlab" {
		return nil, nil
	}
	if s.publicURL == "" {
		return nil, errPublicURLRequired
	}
	projectID, err := strconv.ParseInt(repositoryID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("gitlab project id %q: %w", repositoryID, err)
	}
	client, err := s.gitLabClient(row)
	if err != nil {
		return nil, err
	}
	secret, err := s.GitLabWebhookSecret(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	hookURL := s.publicURL + "/api/v1/webhooks/gitlab/" + connectionID.String()
	hookID, err := client.CreateProjectHook(ctx, projectID, hookURL, secret)
	if err != nil {
		return nil, fmt.Errorf("%w (%w)", errProviderUnavailable, err)
	}
	id := strconv.FormatInt(hookID, 10)
	return &id, nil
}

// RemoveWebhook deletes a hook created by EnsureWebhook. A nil hookID is a no-op.
func (s *Service) RemoveWebhook(ctx context.Context, connectionID uuid.UUID, repositoryID string, hookID *string) error {
	if hookID == nil {
		return nil
	}
	row, err := s.queries.GetGitConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("get git connection: %w", err)
	}
	client, err := s.gitLabClient(row)
	if err != nil {
		return err
	}
	projectID, errP := strconv.ParseInt(repositoryID, 10, 64)
	id, errH := strconv.ParseInt(*hookID, 10, 64)
	if errP != nil || errH != nil {
		return fmt.Errorf("invalid stored hook reference %q/%q", repositoryID, *hookID)
	}
	return client.DeleteProjectHook(ctx, projectID, id)
}
