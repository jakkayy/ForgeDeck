package gitconn

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/safehttp"
	"github.com/jakkayy/ForgeDeck/internal/securetoken"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

const defaultGitLabURL = "https://gitlab.com"

// GitLabInput connects a project with a GitLab access token.
type GitLabInput struct {
	BaseURL string
	Token   string
}

// ConnectGitLab verifies the token with GitLab and stores it encrypted
func (s *Service) ConnectGitLab(ctx context.Context, userID, projectID uuid.UUID, in GitLabInput) (Connection, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ManageGit); err != nil {
		return Connection{}, err
	}
	baseURL, err := s.normalizeGitLabURL(in.BaseURL)
	if err != nil {
		return Connection{}, err
	}
	token := strings.TrimSpace(in.Token)
	if token == "" {
		return Connection{}, validate.Errors{"token": "Paste a GitLab project or group access token."}
	}

	username, err := s.verifyGitLabToken(ctx, baseURL, token)
	if err != nil {
		return Connection{}, err
	}

	id := uuid.New()
	webhookSecret, _ := securetoken.New("")
	tokenCiphertext, tokenKey := s.keyring.Seal([]byte(token), connectionAAD(id, "token"))
	webhookCiphertext, webhookKey := s.keyring.Seal([]byte(webhookSecret), connectionAAD(id, "webhook"))
	host, _ := url.Parse(baseURL)

	row, err := s.queries.CreateGitLabConnection(ctx, store.CreateGitLabConnectionParams{
		ID:                id,
		ProjectID:         projectID,
		DisplayName:       username + " @ " + host.Host,
		BaseUrl:           baseURL,
		TokenCiphertext:   tokenCiphertext,
		TokenKeyID:        &tokenKey,
		WebhookCiphertext: webhookCiphertext,
		WebhookKeyID:      &webhookKey,
		CreatedBy:         &userID,
	})
	if err != nil {
		return Connection{}, fmt.Errorf("store gitlab connection: %w", err)
	}
	return toConnection(row), nil
}

func (s *Service) verifyGitLabToken(ctx context.Context, baseURL, token string) (string, error) {
	client := gitlab.NewClient(s.outbound, baseURL, token)
	info, err := client.Token(ctx)
	switch {
	case errors.Is(err, gitlab.ErrUnauthorized):
		return "", validate.Errors{"token": "GitLab rejected this token. Check that it is active and not expired."}
	case errors.Is(err, safehttp.ErrBlockedDestination):
		return "", validate.Errors{"base_url": "This address is on a private network. An administrator can allow it with FORGEDECK_ALLOW_PRIVATE_GIT_HOSTS."}
	case err != nil:
		return "", validate.Errors{"base_url": "Could not reach GitLab at this address: " + err.Error()}
	}
	if !info.Active || !slices.Contains(info.Scopes, "api") {
		return "", validate.Errors{"token": "The token needs the api scope (to read repositories and register webhooks) and must be active."}
	}
	user, err := client.CurrentUser(ctx)
	if err != nil {
		return "", fmt.Errorf("read gitlab user: %w", err)
	}
	return user.Username, nil
}

// normalizeGitLabURL accepts an https origin; http only when private hosts are allowed
func (s *Service) normalizeGitLabURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return defaultGitLabURL, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", validate.Errors{"base_url": "Use the GitLab address, e.g. https://gitlab.example.com."}
	}
	insecureOK := s.outboundPolicy.AllowPrivate || s.outboundPolicy.AllowLoopback
	schemeOK := u.Scheme == "https" || (u.Scheme == "http" && insecureOK)
	if !schemeOK || u.Host == "" || u.User != nil || u.RawQuery != "" {
		return "", validate.Errors{"base_url": "Use the GitLab address, e.g. https://gitlab.example.com."}
	}
	return u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), nil
}

// connectionAAD binds a credential's ciphertext to its connection and purpose.
func connectionAAD(id uuid.UUID, purpose string) []byte {
	return []byte("git_connection:" + id.String() + ":" + purpose)
}
