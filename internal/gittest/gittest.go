// Package gittest serves real git repositories over HTTP behind the fake
// GitLab API, so tests can clone and fetch commits like a Runner does. It
// needs git and git-http-backend; tests are skipped when they are missing.
package gittest

import (
	"net/http"
	"net/http/cgi" //nolint:gosec // G504 (httpoxy) is fixed in every Go version this project supports
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
)

const backend = "/usr/lib/git-core/git-http-backend"

// Server is one URL that answers both the GitLab API (from the fake) and git
// clones. Clones need the token, like a private repository.
type Server struct {
	URL  string
	root string
}

// New starts a Server in front of fake. Requests to a repository need
// token as the HTTP Basic password; everything else goes to the fake API.
func New(t testing.TB, fake *gitlabtest.Server, token string) *Server {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	if _, err := os.Stat(backend); err != nil {
		t.Skip("git-http-backend is not installed")
	}
	s := &Server{root: t.TempDir()}
	git := &cgi.Handler{Path: backend, Env: []string{"GIT_PROJECT_ROOT=" + s.root, "GIT_HTTP_EXPORT_ALL=1"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ".git/") {
			fake.Config.Handler.ServeHTTP(w, r)
			return
		}
		if _, password, ok := r.BasicAuth(); !ok || password != token {
			w.Header().Set("WWW-Authenticate", "Basic")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		git.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	s.URL = server.URL
	return s
}

// AddRepo creates the repository path (such as "acme/shop") with files in
// one initial commit and returns the commit's SHA. It is served at
// <URL>/<path>.git.
func (s *Server) AddRepo(t testing.TB, path string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(s.root, path+".git")
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	runGit(t, dir, "init", "--quiet", "--initial-branch=main")
	return s.Commit(t, path, files, "Initial commit")
}

// Commit writes files into the repository (replacing same-named files) and
// commits them, returning the new SHA.
func (s *Server) Commit(t testing.TB, path string, files map[string]string, message string) string {
	t.Helper()
	dir := filepath.Join(s.root, path+".git")
	for name, content := range files {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}
	runGit(t, dir, "add", "--all")
	runGit(t, dir, "commit", "--quiet", "--allow-empty", "--message", message)
	return runGit(t, dir, "rev-parse", "HEAD")
}

func runGit(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // test helper
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=dev", "GIT_AUTHOR_EMAIL=dev@example.com",
		"GIT_COMMITTER_NAME=dev", "GIT_COMMITTER_EMAIL=dev@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
