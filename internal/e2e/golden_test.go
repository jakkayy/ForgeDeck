// Package e2e holds tests that run ForgeDeck's components together, with real
// Docker, git and a registry. They run only when FORGEDECK_TEST_DOCKER=1.
package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agent"
	"github.com/jakkayy/ForgeDeck/internal/agent/compose"
	"github.com/jakkayy/ForgeDeck/internal/agent/docker"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/applications"
	"github.com/jakkayy/ForgeDeck/internal/deployments"
	"github.com/jakkayy/ForgeDeck/internal/dockertest"
	"github.com/jakkayy/ForgeDeck/internal/environments"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
	"github.com/jakkayy/ForgeDeck/internal/gittest"
	"github.com/jakkayy/ForgeDeck/internal/registries"
	"github.com/jakkayy/ForgeDeck/internal/runner"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

const (
	dockerfile = "FROM busybox:1.37\nCOPY index.html /www/index.html\nCMD [\"httpd\", \"-f\", \"-p\", \"8080\", \"-h\", \"/www\"]\n"
	// brokenDockerfile runs, but never listens: it passes the build and its
	// tests and must fail the deployment's health check.
	brokenDockerfile = "FROM busybox:1.37\nCOPY index.html /www/index.html\nCMD [\"sleep\", \"3600\"]\n"
	token            = "glpat-golden"
)

// stack is a running ForgeDeck: Control Plane, Runner, and an Agent that
// manages the local Docker.
type stack struct {
	t         *testing.T
	env       *apitest.Env
	owner     *apitest.Client
	git       *gittest.Server
	fake      *gitlabtest.Server
	app       applications.Application
	staging   environments.Environment
	hookPath  string
	hookToken string
	port      int
}

func startStack(t *testing.T) *stack {
	t.Helper()
	dockertest.Require(t)
	fake := gitlabtest.New(t)
	fake.AddToken(token, gitlabtest.Token{Username: "bot", Scopes: []string{"api"}, Projects: []gitlab.Project{
		{ID: 77, PathWithNamespace: "acme/shop", DefaultBranch: "main", Visibility: "private"},
	}})
	git := gittest.New(t, fake, token)
	git.AddRepo(t, "acme/shop", map[string]string{"Dockerfile": dockerfile, "index.html": "v1\n"})
	registryHost := dockertest.StartRegistry(t)
	// The Agent pulls every released image onto this machine; remove them afterwards.
	t.Cleanup(func() {
		out, _ := exec.CommandContext(context.Background(), "docker", "images", "--quiet", registryHost+"/acme/shop").Output() //nolint:gosec // test
		for _, id := range strings.Fields(string(out)) {
			_ = exec.CommandContext(context.Background(), "docker", "image", "rm", "--force", id).Run() //nolint:gosec // test
		}
	})

	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	base := "/api/v1/projects/" + projectID.String()

	var conn gitconn.Connection
	owner.Do(http.MethodPost, base+"/git-connections/gitlab", map[string]string{"base_url": git.URL, "token": token}).
		RequireStatus(http.StatusCreated).Decode(&conn)
	var registry registries.Registry
	owner.Do(http.MethodPost, base+"/registries", map[string]string{"name": "Local", "host": registryHost, "username": "bot", "password": "pw"}).
		RequireStatus(http.StatusCreated).Decode(&registry)
	var app applications.Application
	owner.Do(http.MethodPost, base+"/applications", map[string]any{
		"name": "shop", "git_connection_id": conn.ID, "repository_id": "77", "branch": "main",
		"registry_connection_id": registry.ID, "image_repository": "acme/shop",
		"test_command": "test -s /www/index.html && echo tests-passed",
	}).RequireStatus(http.StatusCreated).Decode(&app)

	// The Agent runs on this machine and manages its Docker.
	enrolled := env.EnrollAgent(owner, projectID, "prod-01")
	engine, err := docker.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	quiet := slog.New(slog.DiscardHandler)
	identity := agent.Identity{ControlPlaneURL: env.Server.URL, AgentID: enrolled.AgentID, ServerID: enrolled.ServerID, Credential: enrolled.Credential}
	agentState, err := agent.NewState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handlers := map[string]worker.Handler{agentproto.JobDeployImage: agent.DeployImageHandler(t.TempDir())}
	runInBackground(t, func(ctx context.Context) error {
		return agent.Run(ctx, identity, agentState, "test", handlers, engine, quiet)
	})

	// The Runner builds, tests and pushes.
	registered := env.NewRunner("build-01")
	runnerCfg := runner.Config{ControlPlaneURL: env.Server.URL, Credential: registered.Credential, Version: "test"}
	build := runner.Handlers(runner.BuildConfig{WorkDir: t.TempDir(), GitProtocols: "http", Timeout: 3 * time.Minute, TestLimits: runner.DefaultTestLimits})
	runInBackground(t, func(ctx context.Context) error { return runner.Run(ctx, runnerCfg, build, runner.DockerStatus, quiet) })

	s := &stack{t: t, env: env, owner: owner, git: git, fake: fake, app: app, port: freePort(t),
		hookPath: "/api/v1/webhooks/gitlab/" + conn.ID.String(), hookToken: fake.Hooks()[0].Token}

	// A staging environment that deploys every successful build, with an HTTP
	// health check.
	container := 8080
	owner.Do(http.MethodPost, "/api/v1/applications/"+app.ID.String()+"/environments", map[string]any{
		"name": "staging", "server_id": enrolled.ServerID, "auto_deploy": true,
		"config": map[string]any{"container_port": container, "published_port": s.port,
			"health_check": map[string]any{"type": "http", "path": "/", "timeout_seconds": 15, "interval_seconds": 1}},
	}).RequireStatus(http.StatusCreated).Decode(&s.staging)
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), "docker", "compose", "--project-name", compose.ProjectName(s.staging.ID), "down", "--volumes").Run() //nolint:gosec // test
	})
	return s
}

// runInBackground runs fn until the test ends and requires it to stop cleanly.
func runInBackground(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- fn(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("background worker: %v", err)
		}
	})
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// push commits files to the repository and tells ForgeDeck about it the way
// GitLab does, with a webhook.
func (s *stack) push(files map[string]string, message string) string {
	s.t.Helper()
	sha := s.git.Commit(s.t, "acme/shop", files, message)
	payload, _ := json.Marshal(map[string]any{
		"ref": "refs/heads/main", "after": sha, "project": map[string]any{"id": 77},
		"commits": []map[string]string{{"id": sha, "message": message}},
	})
	s.env.Anonymous().DoRaw(http.MethodPost, s.hookPath, payload, map[string]string{
		"Content-Type": "application/json", "X-Gitlab-Token": s.hookToken,
		"X-Gitlab-Event": "Push Hook", "X-Gitlab-Event-UUID": uuid.NewString(),
	}).RequireStatus(http.StatusAccepted)
	return sha
}

// deploymentOf waits until the deployment of commit has finished.
func (s *stack) deploymentOf(commit string) deployments.Deployment {
	s.t.Helper()
	deadline := time.Now().Add(4 * time.Minute)
	for {
		var list struct{ Items []deployments.Deployment }
		s.owner.Do(http.MethodGet, "/api/v1/environments/"+s.staging.ID.String()+"/deployments", nil).RequireStatus(http.StatusOK).Decode(&list)
		for _, d := range list.Items {
			if d.CommitSHA == commit && (d.Status == "succeeded" || d.Status == "failed") {
				return d
			}
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("no finished deployment of %s: %+v", commit, list.Items)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *stack) releases() []deployments.Release {
	s.t.Helper()
	var list struct{ Items []deployments.Release }
	s.owner.Do(http.MethodGet, "/api/v1/environments/"+s.staging.ID.String()+"/releases", nil).RequireStatus(http.StatusOK).Decode(&list)
	return list.Items
}

// served returns what the deployed service answers on its published port.
func (s *stack) served() string {
	s.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		req, _ := http.NewRequestWithContext(s.t.Context(), http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", s.port), nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return strings.TrimSpace(string(body))
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("nothing answers on port %d", s.port)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *stack) logOf(d deployments.Deployment) string {
	s.t.Helper()
	var page struct{ Items []struct{ Message string } }
	s.owner.Do(http.MethodGet, "/api/v1/jobs/"+d.JobID.String()+"/logs?limit=1000", nil).RequireStatus(http.StatusOK).Decode(&page)
	var lines []string
	for _, item := range page.Items {
		lines = append(lines, item.Message)
	}
	return strings.Join(lines, "\n")
}

// TestGoldenPath is the product's promise in one test: push code, and it is
// built, tested, shipped, health-checked and recorded; a bad version does not
// replace a good one; and an old release comes back with one request.
func TestGoldenPath(t *testing.T) {
	s := startStack(t)

	// 1. Push v1: webhook → build → test → push image → deploy → healthy.
	v1 := s.push(map[string]string{"index.html": "v1\n"}, "Ship v1")
	first := s.deploymentOf(v1)
	if first.Status != "succeeded" || first.ReleaseID == nil {
		t.Fatalf("v1 deployment %+v\n%s", first, s.logOf(first))
	}
	if got := s.served(); got != "v1" {
		t.Fatalf("service answers %q, want v1", got)
	}
	releases := s.releases()
	if len(releases) != 1 || !releases[0].Current || releases[0].CommitSHA != v1 || !strings.Contains(releases[0].ImageRef, "@sha256:") {
		t.Fatalf("releases after v1: %+v", releases)
	}
	v1Release := releases[0]

	// 2. Push v2: the same path again; v2 replaces v1 and is the current release.
	v2 := s.push(map[string]string{"index.html": "v2\n"}, "Ship v2")
	if d := s.deploymentOf(v2); d.Status != "succeeded" {
		t.Fatalf("v2 deployment %+v\n%s", d, s.logOf(d))
	}
	if got := s.served(); got != "v2" {
		t.Fatalf("service answers %q, want v2", got)
	}
	if releases := s.releases(); len(releases) != 2 || releases[0].CommitSHA != v2 || !releases[0].Current {
		t.Fatalf("releases after v2: %+v", releases)
	}

	// 3. Push a version that builds and passes its tests but never serves:
	// the deployment fails its health check, v2 keeps running, no release.
	v3 := s.push(map[string]string{"Dockerfile": brokenDockerfile, "index.html": "v3\n"}, "Break the service")
	failed := s.deploymentOf(v3)
	if failed.Status != "failed" || failed.Error == nil || failed.Error.Code != "health_check_failed" ||
		failed.Stage == nil || *failed.Stage != agentproto.StageHealthCheck || !strings.Contains(failed.Error.Message, "previous release was restored") {
		t.Fatalf("v3 deployment %+v\n%s", failed, s.logOf(failed))
	}
	if got := s.served(); got != "v2" {
		t.Fatalf("after the failed deployment the service answers %q, want v2 restored", got)
	}
	if releases := s.releases(); len(releases) != 2 || releases[0].CommitSHA != v2 {
		t.Fatalf("a failed deployment changed the releases: %+v", releases)
	}
	if log := s.logOf(failed); !strings.Contains(log, "Last output of the failed service") {
		t.Fatalf("the failed deployment's log lacks the service output:\n%s", log)
	}

	// 4. Roll back to v1: its exact image and configuration, no rebuild.
	var rollback deployments.Deployment
	s.owner.Do(http.MethodPost, "/api/v1/releases/"+v1Release.ID.String()+"/rollback", nil).RequireStatus(http.StatusAccepted).Decode(&rollback)
	deadline := time.Now().Add(3 * time.Minute)
	for rollback.Status != "succeeded" {
		if rollback.Status == "failed" || time.Now().After(deadline) {
			t.Fatalf("rollback %+v\n%s", rollback, s.logOf(rollback))
		}
		time.Sleep(500 * time.Millisecond)
		s.owner.Do(http.MethodGet, "/api/v1/deployments/"+rollback.ID.String(), nil).RequireStatus(http.StatusOK).Decode(&rollback)
	}
	if got := s.served(); got != "v1" {
		t.Fatalf("after the rollback the service answers %q, want v1", got)
	}
	final := s.releases()
	if len(final) != 3 || !final[0].Current || final[0].ImageRef != v1Release.ImageRef || final[0].CommitSHA != v1 {
		t.Fatalf("releases after the rollback: %+v", final)
	}

	// One managed container serves the environment throughout.
	out, err := exec.CommandContext(t.Context(), "docker", "ps", "--quiet", "--filter", "label="+compose.LabelEnvironment+"="+s.staging.ID.String()).Output() //nolint:gosec // test
	if err != nil || len(strings.Fields(string(out))) != 1 {
		t.Fatalf("managed containers %q, %v", out, err)
	}
}
