package gitconn

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
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

// ErrRepositoryNotFound means the connection cannot read the repository.
var ErrRepositoryNotFound = apperr.NotFound("The connection cannot access this repository.")

// FindRepository returns one repository the connection can read, checking
// that the connection belongs to projectID.
func (s *Service) FindRepository(ctx context.Context, userID, projectID, connectionID uuid.UUID, repositoryID string) (Repository, error) {
	row, err := s.authorize(ctx, userID, connectionID, authz.ManageGit)
	if err != nil {
		return Repository{}, err
	}
	if row.ProjectID != projectID {
		return Repository{}, errConnectionNotFound
	}
	repos, err := s.Repositories(ctx, userID, connectionID)
	if err != nil {
		return Repository{}, err
	}
	for _, repo := range repos {
		if repo.ID == repositoryID {
			return repo, nil
		}
	}
	return Repository{}, ErrRepositoryNotFound
}

// CloneCredentials returns an HTTPS clone URL and a short-lived (GitHub) or
// stored (GitLab) credential that can read one repository. It does no user
// authorization; it is for handing a build to a Runner.
func (s *Service) CloneCredentials(ctx context.Context, connectionID uuid.UUID, repositoryID, fullName string) (agentproto.GitCredentials, error) {
	row, err := s.queries.GetGitConnection(ctx, connectionID)
	if err != nil {
		return agentproto.GitCredentials{}, fmt.Errorf("get git connection: %w", err)
	}
	cloneURL := strings.TrimRight(row.BaseUrl, "/") + "/" + fullName + ".git"
	switch row.Provider {
	case "github":
		if s.github == nil {
			return agentproto.GitCredentials{}, errGitHubNotConfigured
		}
		id, err := strconv.ParseInt(repositoryID, 10, 64)
		if err != nil {
			return agentproto.GitCredentials{}, fmt.Errorf("github repository id %q: %w", repositoryID, err)
		}
		token, err := s.github.RepositoryReadToken(ctx, *row.InstallationID, id)
		if err != nil {
			return agentproto.GitCredentials{}, err
		}
		return agentproto.GitCredentials{URL: cloneURL, Username: "x-access-token", Password: token}, nil
	case "gitlab":
		token, err := s.keyring.Open(row.TokenCiphertext, *row.TokenKeyID, connectionAAD(row.ID, "token"))
		if err != nil {
			return agentproto.GitCredentials{}, fmt.Errorf("decrypt gitlab token: %w", err)
		}
		return agentproto.GitCredentials{URL: cloneURL, Username: "oauth2", Password: string(token)}, nil
	default:
		return agentproto.GitCredentials{}, fmt.Errorf("unknown provider %q", row.Provider)
	}
}
