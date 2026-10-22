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
	"github.com/jakkayy/ForgeDeck/internal/registries"
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
	var registry registries.Registry
	owner.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/registries",
		map[string]string{"name": "GHCR", "host": "ghcr.io", "username": "acme-bot", "password": "ghp_push"}).
		RequireStatus(http.StatusCreated).Decode(&registry)
	createApp := func(body map[string]any) applications.Application {
		body["git_connection_id"], body["repository_id"] = conn.ID, "77"
		var app applications.Application
		owner.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/applications", body).
			RequireStatus(http.StatusCreated).Decode(&app)
		return app
	}
	app := createApp(map[string]any{"name": "API", "registry_connection_id": registry.ID, "image_repository": "acme/shop-api"})
	noImage := createApp(map[string]any{"name": "Worker"})

	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)
	enqueueBuild := func(appID, connectionID uuid.UUID) uuid.UUID {
		job, err := svc.Enqueue(t.Context(), jobs.NewJob{ProjectID: projectID, Type: agentproto.JobBuild, WorkerKind: jobs.WorkerRunner,
			Payload: agentproto.BuildPayload{ApplicationID: appID, GitConnectionID: connectionID, RepositoryID: "77",
				RepositoryFullName: "acme/shop", Branch: "main", CommitSHA: "0123456789abcdef0123456789abcdef01234567"}})
		if err != nil {
			t.Fatal(err)
		}
		return job.ID
	}
	brokenConnection := enqueueBuild(app.ID, uuid.New())
	missingImage := enqueueBuild(noImage.ID, conn.ID)
	good := enqueueBuild(app.ID, conn.ID)

	runner := env.NewRunner("build-01")
	wait := 0
	var job agentproto.Job
	runner.Do(http.MethodPost, "/api/v1/runner/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).
		RequireStatus(http.StatusOK).Decode(&job)
	if job.ID != good {
		t.Fatalf("claimed %s, want the build whose credentials could be minted", job.ID)
	}
	want := agentproto.BuildCredentials{
		Git:      agentproto.GitCredentials{URL: fake.URL + "/acme/shop.git", Username: "oauth2", Password: "glpat-good"},
		Registry: agentproto.RegistryCredentials{Host: "ghcr.io", Username: "acme-bot", Password: "ghp_push"},
		Image:    "ghcr.io/acme/shop-api",
	}
	if job.Credentials == nil || *job.Credentials != want {
		t.Fatalf("credentials %+v", job.Credentials)
	}

	for id, code := range map[uuid.UUID]string{brokenConnection: "build_credentials_unavailable", missingImage: "no_image_destination"} {
		var failed jobs.Job
		owner.Do(http.MethodGet, "/api/v1/jobs/"+id.String(), nil).RequireStatus(http.StatusOK).Decode(&failed)
		if failed.Status != jobs.StatusFailed || failed.Error == nil || failed.Error.Code != code {
			t.Fatalf("build %s: %+v, want %s", id, failed, code)
		}
	}
	// credentials travel only in the claim response.
	var stored jobs.Job
	owner.Do(http.MethodGet, "/api/v1/jobs/"+good.String(), nil).RequireStatus(http.StatusOK).Decode(&stored)
	if len(stored.Payload) == 0 || strings.Contains(string(stored.Payload), "glpat-good") || strings.Contains(string(stored.Payload), "ghp_push") {
		t.Fatalf("stored payload %s", stored.Payload)
	}
}
