package applications_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/applications"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

type fixture struct {
	env          *apitest.Env
	owner        *apitest.Client
	projectID    uuid.UUID
	connectionID uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	fake := gitlabtest.New(t)
	fake.AddToken("glpat-good", gitlabtest.Token{Username: "bot", Scopes: []string{"api"}, Projects: []gitlab.Project{
		{ID: 77, PathWithNamespace: "acme/shop", DefaultBranch: "main", Visibility: "private"},
	}})
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	var conn gitconn.Connection
	owner.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/git-connections/gitlab",
		map[string]string{"base_url": fake.URL, "token": "glpat-good"}).RequireStatus(http.StatusCreated).Decode(&conn)
	return fixture{env: env, owner: owner, projectID: projectID, connectionID: conn.ID}
}

func (f fixture) create(t *testing.T, body map[string]any) *apitest.Response {
	t.Helper()
	full := map[string]any{"git_connection_id": f.connectionID, "repository_id": "77"}
	for k, v := range body {
		full[k] = v
	}
	return f.owner.Do(http.MethodPost, "/api/v1/projects/"+f.projectID.String()+"/applications", full)
}

func TestCreateApplicationWithDefaults(t *testing.T) {
	f := newFixture(t)
	var app applications.Application
	f.create(t, map[string]any{"name": "API"}).RequireStatus(http.StatusCreated).Decode(&app)

	if app.RepositoryFullName != "acme/shop" || app.Branch != "main" || app.DockerfilePath != "Dockerfile" ||
		app.BuildContext != "." || app.TestCommand != nil {
		t.Fatalf("app %+v", app)
	}

	var list httpx.List[applications.Application]
	f.owner.Do(http.MethodGet, "/api/v1/projects/"+f.projectID.String()+"/applications", nil).
		RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 1 || list.Items[0].ID != app.ID {
		t.Fatalf("list %+v", list.Items)
	}
}

func TestCreateApplicationValidation(t *testing.T) {
	f := newFixture(t)
	f.create(t, map[string]any{"name": "API"}).RequireStatus(http.StatusCreated)

	cases := map[string]map[string]any{
		"duplicate name":          {"name": "API"},
		"path escapes repository": {"name": "x", "dockerfile_path": "../../etc/passwd"},
		"absolute build context":  {"name": "x", "build_context": "/var/run"},
		"invalid branch":          {"name": "x", "branch": "feature..x"},
		"branch with spaces":      {"name": "x", "branch": "my branch"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			f.create(t, body).RequireStatus(http.StatusUnprocessableEntity)
		})
	}
	f.create(t, map[string]any{"name": "x", "repository_id": "999"}).RequireStatus(http.StatusNotFound)
}

func TestUpdateAndDeleteApplication(t *testing.T) {
	f := newFixture(t)
	dev, devID := f.env.NewUser("dev@example.com")
	f.env.AddMember(f.projectID, devID, "developer")
	var app applications.Application
	f.create(t, map[string]any{"name": "API", "test_command": "go test ./..."}).RequireStatus(http.StatusCreated).Decode(&app)
	path := "/api/v1/applications/" + app.ID.String()

	dev.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK)
	dev.Do(http.MethodPatch, path, map[string]string{"branch": "dev"}).RequireStatus(http.StatusForbidden)

	var updated applications.Application
	f.owner.Do(http.MethodPatch, path, map[string]string{"branch": "release/1.2", "dockerfile_path": "./build/Dockerfile", "test_command": ""}).
		RequireStatus(http.StatusOK).Decode(&updated)
	if updated.Branch != "release/1.2" || updated.DockerfilePath != "build/Dockerfile" || updated.TestCommand != nil || updated.Name != "API" {
		t.Fatalf("updated %+v", updated)
	}

	resp := f.owner.Do(http.MethodDelete, "/api/v1/git-connections/"+f.connectionID.String(), nil).RequireStatus(http.StatusConflict)
	if resp.ErrorCode() != "connection_in_use" {
		t.Fatalf("code %s", resp.ErrorCode())
	}
	f.owner.Do(http.MethodDelete, path, nil).RequireStatus(http.StatusNoContent)
	f.owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusNotFound)
	f.owner.Do(http.MethodDelete, "/api/v1/git-connections/"+f.connectionID.String(), nil).RequireStatus(http.StatusNoContent)
}

func TestApplicationsInvisibleAcrossProjects(t *testing.T) {
	f := newFixture(t)
	var app applications.Application
	f.create(t, map[string]any{"name": "API"}).RequireStatus(http.StatusCreated).Decode(&app)
	outsider, _ := f.env.NewUser("outsider@example.com")
	ownProject := outsider.CreateProject("Mine")

	outsider.Do(http.MethodGet, "/api/v1/applications/"+app.ID.String(), nil).RequireStatus(http.StatusNotFound)
	// Using another project's connection from your own project is refused too.
	outsider.Do(http.MethodPost, "/api/v1/projects/"+ownProject.String()+"/applications",
		map[string]any{"name": "steal", "git_connection_id": f.connectionID, "repository_id": "77"}).
		RequireStatus(http.StatusNotFound)
}
