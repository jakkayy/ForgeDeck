package gitconn

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// Repository is a repository a connection can read, in a provider-neutral
// shape. ID is the provider's own repository ID.
type Repository struct {
	ID            string `json:"id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

var errProviderUnavailable = apperr.Conflict("provider_unavailable",
	"ForgeDeck could not read repositories from the Git provider. Check the connection and try again.")

// Repositories lists what the connection grants access to.
func (s *Service) Repositories(ctx context.Context, userID, connectionID uuid.UUID) ([]Repository, error) {
	row, err := s.authorize(ctx, userID, connectionID, authz.ManageGit)
	if err != nil {
		return nil, err
	}
	switch row.Provider {
	case "github":
		return s.gitHubRepositories(ctx, row)
	case "gitlab":
		return s.gitLabRepositories(ctx, row)
	default:
		return nil, fmt.Errorf("unknown provider %q", row.Provider)
	}
}

func (s *Service) gitHubRepositories(ctx context.Context, row store.GitConnection) ([]Repository, error) {
	if s.github == nil {
		return nil, errGitHubNotConfigured
	}
	repos, err := s.github.Repositories(ctx, *row.InstallationID)
	if err != nil {
		return nil, fmt.Errorf("%w (%w)", errProviderUnavailable, err)
	}
	out := make([]Repository, len(repos))
	for i, r := range repos {
		out[i] = Repository{ID: strconv.FormatInt(r.ID, 10), FullName: r.FullName, DefaultBranch: r.DefaultBranch, Private: r.Private}
	}
	return out, nil
}

func (s *Service) gitLabRepositories(ctx context.Context, row store.GitConnection) ([]Repository, error) {
	client, err := s.gitLabClient(row)
	if err != nil {
		return nil, err
	}
	projects, err := client.Projects(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w (%w)", errProviderUnavailable, err)
	}
	out := make([]Repository, len(projects))
	for i, p := range projects {
		out[i] = Repository{
			ID: strconv.FormatInt(p.ID, 10), FullName: p.PathWithNamespace,
			DefaultBranch: p.DefaultBranch, Private: p.Visibility != "public",
		}
	}
	return out, nil
}

// gitLabClient decrypts the connection's token only for the call being made.
func (s *Service) gitLabClient(row store.GitConnection) (*gitlab.Client, error) {
	token, err := s.keyring.Open(row.TokenCiphertext, *row.TokenKeyID, connectionAAD(row.ID, "token"))
	if err != nil {
		return nil, fmt.Errorf("decrypt gitlab token: %w", err)
	}
	return gitlab.NewClient(s.outbound, row.BaseUrl, string(token)), nil
}
