package runnerapi_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/applications"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

func TestBuildClaimCarriesCloneCredentials(t *testing.T) {
	fake := gitlabtest.New(t)
	fake.AddToken("glpat-good", gitlabtest.Token{Username: "bot", Scopes: []string{"api"}, Projects: []gitlab.Project{
		{ID: 77, PathWithNamespace: "acme/shop", DefaultBranch: "main", Visibility: "private"},
	}})
	env := apitest.New(t, func(cfg *config.Config) { cfg.PublicURL = "https://forgedeck.example.com" })
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	var conn gitconn.Connection
	owner.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/git-connections/gitlab",
		map[string]string{"base_url": fake.URL, "token": "glpat-good"}).RequireStatus(http.StatusCreated).Decode(&conn)
	var app applications.Application
	owner.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/applications",
		map[string]any{"name": "API", "git_connection_id": conn.ID, "repository_id": "77"}).
		RequireStatus(http.StatusCreated).Decode(&app)

	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)
	enqueueBuild := func(connectionID uuid.UUID) uuid.UUID {
		job, err := svc.Enqueue(t.Context(), jobs.NewJob{ProjectID: projectID, Type: agentproto.JobBuild, WorkerKind: jobs.WorkerRunner,
			Payload: agentproto.BuildPayload{ApplicationID: app.ID, GitConnectionID: connectionID, RepositoryID: "77",
				RepositoryFullName: "acme/shop", Branch: "main", CommitSHA: "0123456789abcdef0123456789abcdef01234567"}})
		if err != nil {
			t.Fatal(err)
		}
		return job.ID
	}
	broken := enqueueBuild(uuid.New())
	good := enqueueBuild(conn.ID)

	runner := env.NewRunner("build-01")
	wait := 0
	var job agentproto.Job
	runner.Do(http.MethodPost, "/api/v1/runner/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).
		RequireStatus(http.StatusOK).Decode(&job)
	if job.ID != good {
		t.Fatalf("claimed %s, want the build whose credentials could be minted", job.ID)
	}
	want := agentproto.GitCredentials{URL: fake.URL + "/acme/shop.git", Username: "oauth2", Password: "glpat-good"}
	if job.Credentials == nil || job.Credentials.Git != want {
		t.Fatalf("credentials %+v", job.Credentials)
	}

	var failed jobs.Job
	owner.Do(http.MethodGet, "/api/v1/jobs/"+broken.String(), nil).RequireStatus(http.StatusOK).Decode(&failed)
	if failed.Status != jobs.StatusFailed || failed.Error == nil || failed.Error.Code != "build_credentials_unavailable" {
		t.Fatalf("broken build %+v", failed)
	}
	// Credentials travel only in the claim response.
	var stored jobs.Job
	owner.Do(http.MethodGet, "/api/v1/jobs/"+good.String(), nil).RequireStatus(http.StatusOK).Decode(&stored)
	if len(stored.Payload) == 0 || strings.Contains(string(stored.Payload), "glpat-good") {
		t.Fatalf("stored payload %s", stored.Payload)
	}
}
