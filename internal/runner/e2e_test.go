package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/applications"
	"github.com/jakkayy/ForgeDeck/internal/builds"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/dockertest"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
	"github.com/jakkayy/ForgeDeck/internal/gittest"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/registries"
)

type buildFixture struct {
	env       *apitest.Env
	owner     *apitest.Client
	projectID uuid.UUID
	connID    uuid.UUID
	registry  registries.Registry
	host      string
}

func newBuildFixture(t *testing.T, repos map[string]map[string]string) (buildFixture, map[string]string) {
	t.Helper()
	dockertest.Require(t)
	fake := gitlabtest.New(t)
	fake.AddToken("glpat-good", gitlabtest.Token{Username: "bot", Scopes: []string{"api"}, Projects: []gitlab.Project{
		{ID: 1, PathWithNamespace: "acme/good", DefaultBranch: "main", Visibility: "private"},
		{ID: 2, PathWithNamespace: "acme/bad", DefaultBranch: "main", Visibility: "private"},
	}})
	git := gittest.New(t, fake, "glpat-good")
	commits := map[string]string{}
	for path, files := range repos {
		sha := git.AddRepo(t, path, files)
		commits[path] = sha
		fake.AddCommit(map[string]int64{"acme/good": 1, "acme/bad": 2}[path], "main", gitlab.Commit{ID: sha, Message: "Sample commit"})
	}
	url := git.URL
	host := dockertest.StartRegistry(t)

	env := apitest.New(t, func(cfg *config.Config) { cfg.PublicURL = "https://forgedeck.example.com" })
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	base := "/api/v1/projects/" + projectID.String()
	var conn gitconn.Connection
	owner.Do(http.MethodPost, base+"/git-connections/gitlab", map[string]string{"base_url": url, "token": "glpat-good"}).
		RequireStatus(http.StatusCreated).Decode(&conn)
	var registry registries.Registry
	owner.Do(http.MethodPost, base+"/registries", map[string]string{"name": "Local", "host": host, "username": "bot", "password": "pw"}).
		RequireStatus(http.StatusCreated).Decode(&registry)

	registered := env.NewRunner("build-01")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	cfg := Config{ControlPlaneURL: env.Server.URL, Credential: registered.Credential, Version: "test"}
	handlers := Handlers(BuildConfig{WorkDir: t.TempDir(), GitProtocols: "http", Timeout: 3 * time.Minute,
		TestLimits: DefaultTestLimits})
	go func() { done <- Run(ctx, cfg, handlers, DockerStatus, slog.New(slog.DiscardHandler)) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return buildFixture{env: env, owner: owner, projectID: projectID, connID: conn.ID, registry: registry, host: host}, commits
}

func (f buildFixture) build(t *testing.T, repositoryID, name, testCommand string) jobs.Job {
	t.Helper()
	body := map[string]any{"name": name, "git_connection_id": f.connID, "repository_id": repositoryID,
		"registry_connection_id": f.registry.ID, "image_repository": "acme/" + name}
	if testCommand != "" {
		body["test_command"] = testCommand
	}
	var app applications.Application
	f.owner.Do(http.MethodPost, "/api/v1/projects/"+f.projectID.String()+"/applications", body).
		RequireStatus(http.StatusCreated).Decode(&app)
	var queued builds.Queued
	f.owner.Do(http.MethodPost, "/api/v1/applications/"+app.ID.String()+"/builds", nil).
		RequireStatus(http.StatusAccepted).Decode(&queued)

	deadline := time.Now().Add(3 * time.Minute)
	for {
		var job jobs.Job
		f.owner.Do(http.MethodGet, "/api/v1/jobs/"+queued.JobID.String(), nil).RequireStatus(http.StatusOK).Decode(&job)
		if job.Status == jobs.StatusSucceeded || job.Status == jobs.StatusFailed {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("build never finished: %+v", job)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (f buildFixture) logs(t *testing.T, job jobs.Job) string {
	t.Helper()
	var page struct{ Items []jobs.LogLine }
	f.owner.Do(http.MethodGet, "/api/v1/jobs/"+job.ID.String()+"/logs?limit=1000", nil).RequireStatus(http.StatusOK).Decode(&page)
	var lines []string
	for _, line := range page.Items {
		lines = append(lines, line.Message)
	}
	return strings.Join(lines, "\n")
}

// registryHas reports whether the registry holds the repository.
func registryHas(t *testing.T, host, repository string) bool {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+host+"/v2/"+repository+"/tags/list", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// TestSampleRepositoryBuilds drives the whole path: API, Runner, git clone
// over HTTP, build, test, and push to a registry. A repository whose tests
// fail must fail the job and push nothing.
func TestSampleRepositoryBuilds(t *testing.T) {
	dockerfile := "FROM busybox:1.37\nCOPY app.sh /app.sh\nCMD [\"sh\", \"/app.sh\"]\n"
	files := map[string]string{"Dockerfile": dockerfile, "app.sh": "echo hello\n"}
	f, commits := newBuildFixture(t, map[string]map[string]string{"acme/good": files, "acme/bad": files})
	goodSHA := commits["acme/good"]

	good := f.build(t, "1", "good", "sh /app.sh | grep -q hello && echo tests-passed")
	if good.Status != jobs.StatusSucceeded {
		t.Fatalf("good build %+v\n%s", good.Error, f.logs(t, good))
	}
	var result agentproto.BuildResult
	if err := json.Unmarshal(good.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.CommitSHA != goodSHA || !validDigest.MatchString(result.Digest) ||
		result.ImageRef != fmt.Sprintf("%s/acme/good@%s", f.host, result.Digest) {
		t.Fatalf("result %+v", result)
	}
	if !registryHas(t, f.host, "acme/good") {
		t.Fatal("image was not pushed")
	}
	if logs := f.logs(t, good); !strings.Contains(logs, "tests-passed") || strings.Contains(logs, "glpat-good") {
		t.Fatalf("logs:\n%s", logs)
	}

	bad := f.build(t, "2", "bad", "echo about-to-fail; exit 7")
	if bad.Status != jobs.StatusFailed || bad.Error == nil || bad.Error.Code != "tests_failed" {
		t.Fatalf("bad build %+v\n%s", bad, f.logs(t, bad))
	}
	if registryHas(t, f.host, "acme/bad") {
		t.Fatal("an image whose tests failed was pushed")
	}
	if logs := f.logs(t, bad); !strings.Contains(logs, "about-to-fail") {
		t.Fatalf("test output missing from the log:\n%s", logs)
	}
}
