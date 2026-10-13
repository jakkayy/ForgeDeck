package gitconn

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

var errGitHubNotConfigured = apperr.Conflict("github_not_configured",
	"The GitHub App is not configured on this ForgeDeck. An administrator can set it up; see docs/13-git-providers.md.")

// StartGitHubInstall returns the GitHub URL where the user installs the
// ForgeDeck App for projectID.
func (s *Service) StartGitHubInstall(ctx context.Context, userID, projectID uuid.UUID) (string, error) {
	// Authorize first: whether this installation has a GitHub App is not for
	// non-members to learn.
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ManageGit); err != nil {
		return "", err
	}
	if s.github == nil {
		return "", errGitHubNotConfigured
	}
	state := sealState(s.keyring, installState{
		ProjectID: projectID,
		UserID:    userID,
		ExpiresAt: s.now().Add(installStateTTL).Unix(),
	})
	return s.github.InstallURL(state), nil
}

// GitHubCallback is what GitHub sends back to the App's setup URL.
type GitHubCallback struct {
	State          string
	InstallationID string
	Code           string
}

// InstallError is a failed GitHub install. ProjectID is set when known so
// the user can be sent back to the right project.
type InstallError struct {
	ProjectID uuid.UUID
	Code      string
	Err       error
}

func (e *InstallError) Error() string { return e.Code + ": " + e.Err.Error() }
func (e *InstallError) Unwrap() error { return e.Err }

// CompleteGitHubInstall verifies the callback and stores the connection. It
// trusts nothing in the URL on its own: the state proves who started the
// install, and the OAuth code proves that user can access the installation.
func (s *Service) CompleteGitHubInstall(ctx context.Context, cb GitHubCallback) (uuid.UUID, error) {
	if s.github == nil {
		return uuid.Nil, &InstallError{Code: "github_not_configured", Err: errGitHubNotConfigured}
	}
	state, err := openState(s.keyring, cb.State, s.now())
	if err != nil {
		code := "invalid_state"
		if errors.Is(err, errExpiredInstall) {
			code = "expired"
		}
		return uuid.Nil, &InstallError{Code: code, Err: err}
	}
	fail := func(code string, err error) (uuid.UUID, error) {
		return uuid.Nil, &InstallError{ProjectID: state.ProjectID, Code: code, Err: err}
	}

	installationID, err := strconv.ParseInt(cb.InstallationID, 10, 64)
	if err != nil || cb.Code == "" {
		return fail("invalid_callback", errors.New("installation_id and code are required; enable 'Request user authorization (OAuth) during installation' on the App"))
	}
	ok, err := s.github.UserCanAccessInstallation(ctx, cb.Code, installationID)
	if err != nil {
		return fail("github_error", err)
	}
	if !ok {
		return fail("not_your_installation", errors.New("the signed-in GitHub user cannot access this installation"))
	}
	if _, err := s.authz.Require(ctx, state.UserID, state.ProjectID, authz.ManageGit); err != nil {
		return fail("forbidden", err)
	}
	installation, err := s.github.Installation(ctx, installationID)
	if err != nil {
		return fail("github_error", err)
	}

	_, err = s.queries.CreateGitHubConnection(ctx, store.CreateGitHubConnectionParams{
		ProjectID:      state.ProjectID,
		DisplayName:    installation.Account.Login,
		BaseUrl:        s.github.WebURL(),
		InstallationID: &installationID,
		CreatedBy:      &state.UserID,
	})
	if err != nil {
		return fail("internal", fmt.Errorf("store github connection: %w", err))
	}
	return state.ProjectID, nil
}
