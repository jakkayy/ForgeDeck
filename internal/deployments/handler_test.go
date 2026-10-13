package deployments_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/deployments"
	"github.com/jakkayy/ForgeDeck/internal/environments"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

const (
	digest   = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	commit   = "1111111111111111111111111111111111111111"
	imageRef = "ghcr.io/acme/shop-api@" + digest
)

type fixture struct {
	env       *apitest.Env
	owner     *apitest.Client
	projectID uuid.UUID
	appID     uuid.UUID
	server    apitest.Agent
	staging   environments.Environment
	jobs      *jobs.Service
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	appID := env.CreateApplication(owner, projectID, "API")
	server := env.EnrollAgent(owner, projectID, "prod-01")
	var staging environments.Environment
	owner.Do(http.MethodPost, "/api/v1/applications/"+appID.String()+"/environments", map[string]any{
		"name": "staging", "server_id": server.ServerID,
		"config": map[string]any{"container_port": 8080, "published_port": 18080, "env": map[string]string{"LOG_LEVEL": "info"}},
	}).RequireStatus(http.StatusCreated).Decode(&staging)
	return fixture{env: env, owner: owner, projectID: projectID, appID: appID, server: server, staging: staging,
		jobs: jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)}
}

// build adds a build job of the application in the given state.
func (f fixture) build(t *testing.T, appID uuid.UUID, status string, result any) uuid.UUID {
	t.Helper()
	job, err := f.jobs.Enqueue(t.Context(), jobs.NewJob{ProjectID: f.projectID, ApplicationID: &appID,
		Type: agentproto.JobBuild, WorkerKind: jobs.WorkerRunner, Payload: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if _, err := f.env.Pool.Exec(context.Background(), `UPDATE jobs SET status = $2, result = $3 WHERE id = $1`, job.ID, status, encoded); err != nil {
		t.Fatal(err)
	}
	return job.ID
}

func goodResult() agentproto.BuildResult {
	return agentproto.BuildResult{CommitSHA: commit, Image: "ghcr.io/acme/shop-api", Tag: commit[:12], Digest: digest, ImageRef: imageRef}
}

func (f fixture) deploy(client *apitest.Client, environmentID, buildID uuid.UUID) *apitest.Response {
	return client.Do(http.MethodPost, "/api/v1/environments/"+environmentID.String()+"/deployments", map[string]any{"build_job_id": buildID})
}

func TestDeployQueuesJobForTheEnvironmentsServer(t *testing.T) {
	f := newFixture(t)
	buildID := f.build(t, f.appID, "succeeded", goodResult())

	var d deployments.Deployment
	f.deploy(f.owner, f.staging.ID, buildID).RequireStatus(http.StatusAccepted).Decode(&d)
	if d.Status != "queued" || d.ImageRef != imageRef || d.CommitSHA != commit || d.JobID == nil || *d.SourceBuildJobID != buildID {
		t.Fatalf("deployment %+v", d)
	}

	// The job targets the environment's server and carries the digest and config, not secrets.
	var job jobs.Job
	f.owner.Do(http.MethodGet, "/api/v1/jobs/"+d.JobID.String(), nil).RequireStatus(http.StatusOK).Decode(&job)
	var payload agentproto.DeployImagePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if job.Type != agentproto.JobDeployImage || job.TargetServerID == nil || *job.TargetServerID != f.server.ServerID ||
		payload.DeploymentID != d.ID || payload.EnvironmentID != f.staging.ID || payload.ImageRef != imageRef ||
		payload.Config.Env["LOG_LEVEL"] != "info" || *payload.Config.PublishedPort != 18080 {
		t.Fatalf("job %+v payload %+v", job, payload)
	}

	// The Agent of that server can claim it; an Agent of another server cannot.
	other := f.env.EnrollAgent(f.owner, f.projectID, "prod-02")
	wait := 0
	other.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusNoContent)
	var claimed agentproto.Job
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).
		RequireStatus(http.StatusOK).Decode(&claimed)
	if claimed.ID != *d.JobID || claimed.Type != agentproto.JobDeployImage {
		t.Fatalf("claimed %+v", claimed)
	}

	// The deployment follows its job.
	var running deployments.Deployment
	f.owner.Do(http.MethodGet, "/api/v1/deployments/"+d.ID.String(), nil).RequireStatus(http.StatusOK).Decode(&running)
	if running.Status != "running" || running.StartedAt == nil {
		t.Fatalf("running %+v", running)
	}
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/"+claimed.ID.String()+"/complete", agentproto.CompleteRequest{
		Attempt: claimed.Attempt, Outcome: agentproto.OutcomeFailed, ErrorCode: "health_check_failed", ErrorMessage: "did not become healthy",
	}).RequireStatus(http.StatusNoContent)
	var failed deployments.Deployment
	f.owner.Do(http.MethodGet, "/api/v1/deployments/"+d.ID.String(), nil).RequireStatus(http.StatusOK).Decode(&failed)
	if failed.Status != "failed" || failed.Error == nil || failed.Error.Code != "health_check_failed" || failed.FinishedAt == nil {
		t.Fatalf("failed %+v", failed)
	}

	var list struct{ Items []deployments.Deployment }
	f.owner.Do(http.MethodGet, "/api/v1/environments/"+f.staging.ID.String()+"/deployments", nil).RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 1 || list.Items[0].ID != d.ID {
		t.Fatalf("list %+v", list.Items)
	}
}

func TestDeployRejectsUnusableBuilds(t *testing.T) {
	f := newFixture(t)
	otherApp := f.env.CreateApplication(f.owner, f.projectID, "Worker")
	mutableTag := goodResult()
	mutableTag.ImageRef = "ghcr.io/acme/shop-api:latest"

	cases := map[string]uuid.UUID{
		"build still running":     f.build(t, f.appID, "running", nil),
		"build failed":            f.build(t, f.appID, "failed", nil),
		"build of another app":    f.build(t, otherApp, "succeeded", goodResult()),
		"result without a digest": f.build(t, f.appID, "succeeded", mutableTag),
		"job that does not exist": uuid.New(),
	}
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			if resp := f.deploy(f.owner, f.staging.ID, id); resp.Status != http.StatusUnprocessableEntity {
				t.Fatalf("status %d: %s", resp.Status, resp.Body)
			}
		})
	}
	var list struct{ Items []deployments.Deployment }
	f.owner.Do(http.MethodGet, "/api/v1/environments/"+f.staging.ID.String()+"/deployments", nil).RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 0 {
		t.Fatalf("rejected requests left deployments: %+v", list.Items)
	}
}

func TestDeploymentAuthorization(t *testing.T) {
	f := newFixture(t)
	buildID := f.build(t, f.appID, "succeeded", goodResult())
	viewer, viewerID := f.env.NewUser("viewer@example.com")
	f.env.AddMember(f.projectID, viewerID, "viewer")
	developer, developerID := f.env.NewUser("dev@example.com")
	f.env.AddMember(f.projectID, developerID, "developer")
	outsider, _ := f.env.NewUser("outsider@example.com")

	f.deploy(viewer, f.staging.ID, buildID).RequireStatus(http.StatusForbidden)
	f.deploy(outsider, f.staging.ID, buildID).RequireStatus(http.StatusNotFound)
	var d deployments.Deployment
	f.deploy(developer, f.staging.ID, buildID).RequireStatus(http.StatusAccepted).Decode(&d)

	path := "/api/v1/deployments/" + d.ID.String()
	viewer.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK)
	outsider.Do(http.MethodGet, path, nil).RequireStatus(http.StatusNotFound)
	outsider.Do(http.MethodGet, "/api/v1/environments/"+f.staging.ID.String()+"/deployments", nil).RequireStatus(http.StatusNotFound)
}
