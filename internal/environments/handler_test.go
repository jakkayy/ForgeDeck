package environments_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/applications"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/environments"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
)

type fixture struct {
	env       *apitest.Env
	owner     *apitest.Client
	projectID uuid.UUID
	app       applications.Application
	server    apitest.Agent
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	fake := gitlabtest.New(t)
	fake.AddToken("glpat-good", gitlabtest.Token{Username: "bot", Scopes: []string{"api"}, Projects: []gitlab.Project{
		{ID: 77, PathWithNamespace: "acme/shop", DefaultBranch: "main", Visibility: "private"},
	}})
	env := apitest.New(t, func(cfg *config.Config) { cfg.PublicURL = "https://forgedeck.example.com" })
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	base := "/api/v1/projects/" + projectID.String()
	var conn gitconn.Connection
	owner.Do(http.MethodPost, base+"/git-connections/gitlab", map[string]string{"base_url": fake.URL, "token": "glpat-good"}).
		RequireStatus(http.StatusCreated).Decode(&conn)
	var app applications.Application
	owner.Do(http.MethodPost, base+"/applications", map[string]any{"name": "API", "git_connection_id": conn.ID, "repository_id": "77"}).
		RequireStatus(http.StatusCreated).Decode(&app)
	return fixture{env: env, owner: owner, projectID: projectID, app: app, server: env.EnrollAgent(owner, projectID, "prod-01")}
}

func (f fixture) create(client *apitest.Client, body map[string]any) *apitest.Response {
	full := map[string]any{"name": "production", "server_id": f.server.ServerID}
	for k, v := range body {
		full[k] = v
	}
	return client.Do(http.MethodPost, "/api/v1/applications/"+f.app.ID.String()+"/environments", full)
}

func TestCreateEnvironmentAppliesDefaults(t *testing.T) {
	f := newFixture(t)
	var created environments.Environment
	f.create(f.owner, map[string]any{"config": map[string]any{"container_port": 8080, "published_port": 18080}}).
		RequireStatus(http.StatusCreated).Decode(&created)

	cfg := created.Config
	if created.Name != "production" || created.AutoDeploy || cfg.BindAddress != "127.0.0.1" || cfg.Restart != "unless-stopped" ||
		cfg.HealthCheck.Type != "tcp" || cfg.HealthCheck.TimeoutSeconds != 60 {
		t.Fatalf("created %+v", created)
	}

	var list struct{ Items []environments.Environment }
	f.owner.Do(http.MethodGet, "/api/v1/applications/"+f.app.ID.String()+"/environments", nil).
		RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 1 || list.Items[0].ID != created.ID {
		t.Fatalf("list %+v", list.Items)
	}
}

func TestEnvironmentValidation(t *testing.T) {
	f := newFixture(t)
	f.create(f.owner, nil).RequireStatus(http.StatusCreated)

	cases := map[string]map[string]any{
		"duplicate name":       {},
		"bad name":             {"name": "Prod Env"},
		"unsafe volume":        {"name": "a", "config": map[string]any{"volumes": []map[string]string{{"name": "/etc", "path": "/x"}}}},
		"privileged host port": {"name": "b", "config": map[string]any{"container_port": 80, "published_port": 80}},
		"unknown server":       {"name": "c", "server_id": uuid.New()},
		"unknown config field": {"name": "d", "config": map[string]any{"privileged": true}},
		"http without a port":  {"name": "e", "config": map[string]any{"health_check": map[string]any{"type": "http", "path": "/"}}},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			resp := f.create(f.owner, body)
			if resp.Status != http.StatusUnprocessableEntity && resp.Status != http.StatusBadRequest {
				t.Fatalf("status %d: %s", resp.Status, resp.Body)
			}
		})
	}
}

func TestPublishedPortIsUniquePerServer(t *testing.T) {
	f := newFixture(t)
	port := map[string]any{"container_port": 80, "published_port": 18080}
	f.create(f.owner, map[string]any{"name": "production", "config": port}).RequireStatus(http.StatusCreated)
	f.create(f.owner, map[string]any{"name": "staging", "config": port}).RequireStatus(http.StatusUnprocessableEntity)

	// The same port on another server is fine.
	other := f.env.EnrollAgent(f.owner, f.projectID, "prod-02")
	f.create(f.owner, map[string]any{"name": "staging", "server_id": other.ServerID, "config": port}).
		RequireStatus(http.StatusCreated)
}

func TestServerFromAnotherProjectIsRefused(t *testing.T) {
	f := newFixture(t)
	otherProject := f.owner.CreateProject("Other")
	foreign := f.env.EnrollAgent(f.owner, otherProject, "foreign-01")
	f.create(f.owner, map[string]any{"server_id": foreign.ServerID}).RequireStatus(http.StatusUnprocessableEntity)
}

func TestUpdateAndDeleteEnvironment(t *testing.T) {
	f := newFixture(t)
	var created environments.Environment
	f.create(f.owner, map[string]any{"config": map[string]any{"container_port": 80, "published_port": 18080}}).
		RequireStatus(http.StatusCreated).Decode(&created)
	path := "/api/v1/environments/" + created.ID.String()

	var updated environments.Environment
	f.owner.Do(http.MethodPatch, path, map[string]any{"auto_deploy": true}).RequireStatus(http.StatusOK).Decode(&updated)
	if !updated.AutoDeploy || updated.Config.PublishedPort == nil || *updated.Config.PublishedPort != 18080 {
		t.Fatalf("auto_deploy update changed config: %+v", updated)
	}

	f.owner.Do(http.MethodPatch, path, map[string]any{"config": map[string]any{"env": map[string]string{"LOG_LEVEL": "debug"}}}).
		RequireStatus(http.StatusOK).Decode(&updated)
	if updated.Config.PublishedPort != nil || updated.Config.Env["LOG_LEVEL"] != "debug" || !updated.AutoDeploy {
		t.Fatalf("config replace %+v", updated)
	}
	// Dropping the published port frees it for another environment.
	f.create(f.owner, map[string]any{"name": "staging", "config": map[string]any{"container_port": 80, "published_port": 18080}}).
		RequireStatus(http.StatusCreated)

	resp := f.owner.Do(http.MethodDelete, "/api/v1/servers/"+f.server.ServerID.String(), nil).RequireStatus(http.StatusConflict)
	if resp.ErrorCode() != "server_in_use" {
		t.Fatalf("code %s", resp.ErrorCode())
	}
	f.owner.Do(http.MethodDelete, path, nil).RequireStatus(http.StatusNoContent)
	f.owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusNotFound)
}

func TestEnvironmentAuthorization(t *testing.T) {
	f := newFixture(t)
	var created environments.Environment
	f.create(f.owner, nil).RequireStatus(http.StatusCreated).Decode(&created)
	path := "/api/v1/environments/" + created.ID.String()

	developer, developerID := f.env.NewUser("dev@example.com")
	f.env.AddMember(f.projectID, developerID, "developer")
	outsider, _ := f.env.NewUser("outsider@example.com")

	developer.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK)
	f.create(developer, map[string]any{"name": "other"}).RequireStatus(http.StatusForbidden)
	developer.Do(http.MethodPatch, path, map[string]any{"auto_deploy": true}).RequireStatus(http.StatusForbidden)
	developer.Do(http.MethodDelete, path, nil).RequireStatus(http.StatusForbidden)

	outsider.Do(http.MethodGet, path, nil).RequireStatus(http.StatusNotFound)
	outsider.Do(http.MethodGet, "/api/v1/applications/"+f.app.ID.String()+"/environments", nil).RequireStatus(http.StatusNotFound)
	f.create(outsider, nil).RequireStatus(http.StatusNotFound)
}
