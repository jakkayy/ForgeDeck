package builds_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/applications"
	"github.com/jakkayy/ForgeDeck/internal/builds"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/registries"
)

const (
	headSHA  = "1111111111111111111111111111111111111111"
	olderSHA = "2222222222222222222222222222222222222222"
)

type fixture struct {
	env       *apitest.Env
	owner     *apitest.Client
	projectID uuid.UUID
	app       applications.Application
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	fake := gitlabtest.New(t)
	fake.AddToken("glpat-good", gitlabtest.Token{Username: "bot", Scopes: []string{"api"}, Projects: []gitlab.Project{
		{ID: 77, PathWithNamespace: "acme/shop", DefaultBranch: "main", Visibility: "private"},
	}})
	fake.AddCommit(77, "main", gitlab.Commit{ID: headSHA, Message: "Add checkout\n\nLonger description"})
	fake.AddCommit(77, olderSHA, gitlab.Commit{ID: olderSHA, Message: "Initial commit"})

	env := apitest.New(t, func(cfg *config.Config) { cfg.PublicURL = "https://forgedeck.example.com" })
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	base := "/api/v1/projects/" + projectID.String()
	var conn gitconn.Connection
	owner.Do(http.MethodPost, base+"/git-connections/gitlab", map[string]string{"base_url": fake.URL, "token": "glpat-good"}).
		RequireStatus(http.StatusCreated).Decode(&conn)
	var registry registries.Registry
	owner.Do(http.MethodPost, base+"/registries", map[string]string{"name": "GHCR", "host": "ghcr.io", "username": "bot", "password": "secret"}).
		RequireStatus(http.StatusCreated).Decode(&registry)
	var app applications.Application
	owner.Do(http.MethodPost, base+"/applications", map[string]any{"name": "API", "git_connection_id": conn.ID,
		"repository_id": "77", "registry_connection_id": registry.ID, "image_repository": "acme/shop-api", "test_command": "make test"}).
		RequireStatus(http.StatusCreated).Decode(&app)
	return fixture{env: env, owner: owner, projectID: projectID, app: app}
}

func (f fixture) trigger(client *apitest.Client, body any) *apitest.Response {
	return client.Do(http.MethodPost, "/api/v1/applications/"+f.app.ID.String()+"/builds", body)
}

func TestTriggerBuildsBranchHead(t *testing.T) {
	f := newFixture(t)
	var queued builds.Queued
	f.trigger(f.owner, nil).RequireStatus(http.StatusAccepted).Decode(&queued)
	if queued.CommitSHA != headSHA || queued.Status != jobs.StatusQueued {
		t.Fatalf("queued %+v", queued)
	}

	var job jobs.Job
	f.owner.Do(http.MethodGet, "/api/v1/jobs/"+queued.JobID.String(), nil).RequireStatus(http.StatusOK).Decode(&job)
	var payload agentproto.BuildPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if job.Type != agentproto.JobBuild || payload.CommitSHA != headSHA || payload.CommitMessage != "Add checkout" ||
		payload.Branch != "main" || payload.TestCommand == nil || *payload.TestCommand != "make test" {
		t.Fatalf("job %+v payload %+v", job, payload)
	}

	var again builds.Queued
	f.trigger(f.owner, map[string]string{}).RequireStatus(http.StatusAccepted).Decode(&again)
	if again.JobID != queued.JobID {
		t.Fatalf("second trigger queued %s, want %s", again.JobID, queued.JobID)
	}

	f.owner.Do(http.MethodPost, "/api/v1/jobs/"+queued.JobID.String()+"/cancel", nil).RequireStatus(http.StatusOK)
	var rebuilt builds.Queued
	f.trigger(f.owner, nil).RequireStatus(http.StatusAccepted).Decode(&rebuilt)
	if rebuilt.JobID == queued.JobID {
		t.Fatal("rebuild returned the cancelled job")
	}
}

func TestTriggerSpecificCommit(t *testing.T) {
	f := newFixture(t)
	var queued builds.Queued
	f.trigger(f.owner, map[string]string{"commit_sha": strings.ToUpper(olderSHA)}).RequireStatus(http.StatusAccepted).Decode(&queued)
	if queued.CommitSHA != olderSHA {
		t.Fatalf("queued %+v", queued)
	}
	f.trigger(f.owner, map[string]string{"commit_sha": "3333333333333333333333333333333333333333"}).RequireStatus(http.StatusNotFound)
	f.trigger(f.owner, map[string]string{"commit_sha": "main"}).RequireStatus(http.StatusUnprocessableEntity)
}

func TestTriggerAuthorizationAndPreconditions(t *testing.T) {
	f := newFixture(t)
	viewer, viewerID := f.env.NewUser("viewer@example.com")
	f.env.AddMember(f.projectID, viewerID, "viewer")
	developer, developerID := f.env.NewUser("dev@example.com")
	f.env.AddMember(f.projectID, developerID, "developer")
	outsider, _ := f.env.NewUser("outsider@example.com")

	f.trigger(viewer, nil).RequireStatus(http.StatusForbidden)
	f.trigger(outsider, nil).RequireStatus(http.StatusNotFound)
	f.trigger(developer, nil).RequireStatus(http.StatusAccepted)

	f.owner.Do(http.MethodPatch, "/api/v1/applications/"+f.app.ID.String(), map[string]string{"image_repository": ""}).
		RequireStatus(http.StatusOK)
	if code := f.trigger(f.owner, nil).RequireStatus(http.StatusConflict).ErrorCode(); code != "no_image_destination" {
		t.Fatalf("code %s", code)
	}
}
