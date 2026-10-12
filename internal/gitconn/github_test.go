package gitconn_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/github/githubtest"
)

type githubFixture struct {
	env       *apitest.Env
	fake      *githubtest.Server
	owner     *apitest.Client
	projectID uuid.UUID
}

func newGitHubFixture(t *testing.T) githubFixture {
	t.Helper()
	fake := githubtest.New(t)
	fake.AddInstallation(11, "acme")
	fake.AddInstallation(22, "victim-org")
	fake.AddOAuthCode("acme-admin-code", 11)
	env := apitest.New(t, func(cfg *config.Config) {
		ghCfg := fake.Config()
		cfg.GitHub = &ghCfg
		cfg.PublicURL = "https://forgedeck.example.com"
	})
	owner, _ := env.NewUser("owner@example.com")
	return githubFixture{env: env, fake: fake, owner: owner, projectID: owner.CreateProject("Shop")}
}

// startInstall returns the state ForgeDeck put in the GitHub install URL.
func (f githubFixture) startInstall(t *testing.T) string {
	t.Helper()
	var resp struct {
		InstallURL string `json:"install_url"`
	}
	f.owner.Do(http.MethodPost, "/api/v1/projects/"+f.projectID.String()+"/git-connections/github", nil).
		RequireStatus(http.StatusOK).Decode(&resp)
	if !strings.HasPrefix(resp.InstallURL, f.fake.URL+"/apps/forgedeck-test/installations/new?state=") {
		t.Fatalf("install URL %q", resp.InstallURL)
	}
	u, _ := url.Parse(resp.InstallURL)
	return u.Query().Get("state")
}

// callback simulates GitHub redirecting the browser back, without cookies.
func (f githubFixture) callback(t *testing.T, state, installationID, code string) string {
	t.Helper()
	q := url.Values{"state": {state}, "installation_id": {installationID}, "code": {code}, "setup_action": {"install"}}
	resp := f.env.Anonymous().Do(http.MethodGet, "/api/v1/git/github/setup?"+q.Encode(), nil).RequireStatus(http.StatusSeeOther)
	return resp.Header.Get("Location")
}

func (f githubFixture) connections(t *testing.T) []gitconn.Connection {
	t.Helper()
	var list struct {
		Items []gitconn.Connection `json:"items"`
	}
	f.owner.Do(http.MethodGet, "/api/v1/projects/"+f.projectID.String()+"/git-connections", nil).
		RequireStatus(http.StatusOK).Decode(&list)
	return list.Items
}

func TestGitHubInstallCreatesConnection(t *testing.T) {
	f := newGitHubFixture(t)
	state := f.startInstall(t)

	location := f.callback(t, state, "11", "acme-admin-code")
	if location != "/projects/"+f.projectID.String()+"?git=connected" {
		t.Fatalf("redirect %q", location)
	}
	conns := f.connections(t)
	if len(conns) != 1 || conns[0].Provider != "github" || conns[0].DisplayName != "acme" {
		t.Fatalf("connections %+v", conns)
	}

	// Installing again (for example after changing repo access) does not duplicate it.
	f.callback(t, f.startInstall(t), "11", "acme-admin-code")
	if n := len(f.connections(t)); n != 1 {
		t.Fatalf("%d connections after reinstall", n)
	}
}

func TestGitHubInstallRejectsForgedCallbacks(t *testing.T) {
	f := newGitHubFixture(t)
	state := f.startInstall(t)
	project := "/projects/" + f.projectID.String()

	cases := map[string]struct{ state, installation, code, want string }{
		"someone else's installation": {state, "22", "acme-admin-code", project + "?git_error=not_your_installation"},
		"forged OAuth code":           {state, "11", "forged", project + "?git_error=github_error"},
		"missing code":                {state, "11", "", project + "?git_error=invalid_callback"},
		"tampered state":              {state[:len(state)-2] + "AA", "11", "acme-admin-code", "/projects?git_error=invalid_state"},
		"no state":                    {"", "11", "acme-admin-code", "/projects?git_error=invalid_state"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := f.callback(t, tc.state, tc.installation, tc.code); got != tc.want {
				t.Fatalf("redirect %q, want %q", got, tc.want)
			}
		})
	}
	if n := len(f.connections(t)); n != 0 {
		t.Fatalf("forged callbacks created %d connections", n)
	}
}

func TestGitHubInstallRequiresAdmin(t *testing.T) {
	f := newGitHubFixture(t)
	dev, devID := f.env.NewUser("dev@example.com")
	f.env.AddMember(f.projectID, devID, "developer")
	dev.Do(http.MethodPost, "/api/v1/projects/"+f.projectID.String()+"/git-connections/github", nil).
		RequireStatus(http.StatusForbidden)
}

func TestGitHubNotConfigured(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	resp := owner.Do(http.MethodPost, "/api/v1/projects/"+owner.CreateProject("Shop").String()+"/git-connections/github", nil).
		RequireStatus(http.StatusConflict)
	if resp.ErrorCode() != "github_not_configured" {
		t.Fatalf("code %s", resp.ErrorCode())
	}
}

func TestDeleteConnection(t *testing.T) {
	f := newGitHubFixture(t)
	f.callback(t, f.startInstall(t), "11", "acme-admin-code")
	conn := f.connections(t)[0]

	outsider, _ := f.env.NewUser("outsider@example.com")
	outsider.Do(http.MethodDelete, "/api/v1/git-connections/"+conn.ID.String(), nil).RequireStatus(http.StatusNotFound)
	f.owner.Do(http.MethodDelete, "/api/v1/git-connections/"+conn.ID.String(), nil).RequireStatus(http.StatusNoContent)
	if n := len(f.connections(t)); n != 0 {
		t.Fatalf("%d connections after delete", n)
	}
}
