package webhooks_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/applications"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
)

type gitLabProject struct {
	env    *apitest.Env
	app    applications.Application
	path   string
	secret string
}

func newGitLabProject(t *testing.T) gitLabProject {
	t.Helper()
	fake := gitlabtest.New(t)
	fake.AddToken("glpat", gitlabtest.Token{Username: "bot", Scopes: []string{"api"}, Projects: []gitlab.Project{
		{ID: 15, PathWithNamespace: "acme/shop", DefaultBranch: "main"},
	}})
	env := apitest.New(t, func(cfg *config.Config) { cfg.PublicURL = "https://forgedeck.example.com" })
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	var conn gitconn.Connection
	owner.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/git-connections/gitlab",
		map[string]string{"base_url": fake.URL, "token": "glpat"}).RequireStatus(http.StatusCreated).Decode(&conn)
	var app applications.Application
	owner.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/applications",
		map[string]any{"name": "API", "git_connection_id": conn.ID, "repository_id": "15", "test_command": "make test"}).
		RequireStatus(http.StatusCreated).Decode(&app)
	return gitLabProject{env: env, app: app, path: "/api/v1/webhooks/gitlab/" + conn.ID.String(), secret: fake.Hooks()[0].Token}
}

func (p gitLabProject) push(t *testing.T, delivery, branch, commit string) *apitest.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"ref": "refs/heads/" + branch, "after": commit, "project": map[string]int{"id": 15},
		"commits": []map[string]string{{"id": commit, "message": "Ship it\n\ndetails"}},
	})
	return p.env.Anonymous().DoRaw(http.MethodPost, p.path, body, map[string]string{
		"X-Gitlab-Token": p.secret, "X-Gitlab-Event": "Push Hook", "X-Gitlab-Event-UUID": delivery,
	})
}

func buildJobs(t *testing.T, env *apitest.Env) []agentproto.BuildPayload {
	t.Helper()
	rows, err := env.Pool.Query(t.Context(), "SELECT payload FROM jobs WHERE type = 'build' AND worker_kind = 'runner' ORDER BY created_at")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var builds []agentproto.BuildPayload
	for rows.Next() {
		var raw []byte
		var payload agentproto.BuildPayload
		if err := rows.Scan(&raw); err != nil || json.Unmarshal(raw, &payload) != nil {
			t.Fatalf("scan build: %v", err)
		}
		builds = append(builds, payload)
	}
	return builds
}

func TestGitLabPushQueuesBuildForTrackedBranch(t *testing.T) {
	p := newGitLabProject(t)
	const commit = "da1560886d4f094c3e6c9ef40349f7d38b5d27d7"

	p.push(t, "u-1", "main", commit).RequireStatus(http.StatusAccepted)
	builds := buildJobs(t, p.env)
	if len(builds) != 1 {
		t.Fatalf("%d builds, want 1", len(builds))
	}
	b := builds[0]
	if b.ApplicationID != p.app.ID || b.CommitSHA != commit || b.Branch != "main" || b.RepositoryFullName != "acme/shop" ||
		b.CommitMessage != "Ship it" || b.TestCommand == nil || *b.TestCommand != "make test" {
		t.Fatalf("build payload %+v", b)
	}
}

func TestGitLabPushIgnoresOtherBranchesAndRepeats(t *testing.T) {
	p := newGitLabProject(t)
	const commit = "da1560886d4f094c3e6c9ef40349f7d38b5d27d7"

	p.push(t, "u-1", "feature/x", commit).RequireStatus(http.StatusAccepted)
	if n := len(buildJobs(t, p.env)); n != 0 {
		t.Fatalf("untracked branch queued %d builds", n)
	}

	p.push(t, "u-2", "main", commit).RequireStatus(http.StatusAccepted)
	p.push(t, "u-2", "main", commit).RequireStatus(http.StatusOK)
	p.push(t, "u-3", "main", commit).RequireStatus(http.StatusAccepted)
	if n := len(buildJobs(t, p.env)); n != 1 {
		t.Fatalf("same commit queued %d builds, want 1", n)
	}
}

func TestGitHubPushBuildsInEveryConnectedProject(t *testing.T) {
	env := gitHubEnv(t)
	ctx := t.Context()
	for _, slug := range []string{"one", "two"} {
		var projectID, connID uuid.UUID
		if err := env.Pool.QueryRow(ctx, "INSERT INTO projects (name, slug) VALUES ($1, $2) RETURNING id", slug, slug).Scan(&projectID); err != nil {
			t.Fatal(err)
		}
		if err := env.Pool.QueryRow(ctx, `INSERT INTO git_connections (project_id, provider, display_name, base_url, installation_id)
			VALUES ($1, 'github', 'acme', 'https://github.com', 11) RETURNING id`, projectID).Scan(&connID); err != nil {
			t.Fatal(err)
		}
		if _, err := env.Pool.Exec(ctx, `INSERT INTO applications (project_id, name, git_connection_id, repository_id, repository_full_name, branch)
			VALUES ($1, 'api', $2, '5', 'acme/api', 'main')`, projectID, connID); err != nil {
			t.Fatal(err)
		}
	}

	env.Anonymous().DoRaw(http.MethodPost, "/api/v1/webhooks/github", push, signed(push, "push", "d-1")).RequireStatus(http.StatusAccepted)
	if n := len(buildJobs(t, env)); n != 2 {
		t.Fatalf("%d builds, want one per project", n)
	}
}

func TestPushBuildsAreAuditedOncePerBuild(t *testing.T) {
	p := newGitLabProject(t)
	commit := "0123456789abcdef0123456789abcdef01234567"
	p.push(t, "d-1", "main", commit).RequireStatus(http.StatusAccepted)
	p.push(t, "d-2", "main", commit).RequireStatus(http.StatusAccepted)

	rows, err := p.env.Pool.Query(t.Context(), `SELECT actor, target_name, metadata FROM audit_logs WHERE action = 'build.queue'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var events int
	for rows.Next() {
		var actor, name string
		var metadata map[string]any
		if err := rows.Scan(&actor, &name, &metadata); err != nil {
			t.Fatal(err)
		}
		if actor != "webhook" || name != "API" || metadata["commit"] != commit || metadata["branch"] != "main" {
			t.Fatalf("event %s %s %v", actor, name, metadata)
		}
		events++
	}
	if events != 1 {
		t.Fatalf("%d build.queue events for one build", events)
	}
}
