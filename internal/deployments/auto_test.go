package deployments_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/deployments"
	"github.com/jakkayy/ForgeDeck/internal/environments"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
)

type autoFixture struct {
	fixture
	runner       apitest.Runner
	connectionID uuid.UUID
}

func newAutoFixture(t *testing.T) autoFixture {
	t.Helper()
	f := newFixture(t)
	var registry struct{ ID uuid.UUID }
	f.owner.Do(http.MethodPost, "/api/v1/projects/"+f.projectID.String()+"/registries",
		map[string]string{"name": "GHCR", "host": "ghcr.io", "username": "bot", "password": "secret"}).
		RequireStatus(http.StatusCreated).Decode(&registry)
	f.owner.Do(http.MethodPatch, "/api/v1/applications/"+f.appID.String(),
		map[string]any{"registry_connection_id": registry.ID, "image_repository": "acme/shop-api"}).RequireStatus(http.StatusOK)
	var connectionID uuid.UUID
	if err := f.env.Pool.QueryRow(context.Background(), `SELECT id FROM git_connections WHERE project_id = $1`, f.projectID).Scan(&connectionID); err != nil {
		t.Fatal(err)
	}
	return autoFixture{fixture: f, runner: f.env.NewRunner("build-01"), connectionID: connectionID}
}

func (f autoFixture) setAutoDeploy(t *testing.T, environmentID uuid.UUID, on bool) {
	t.Helper()
	f.owner.Do(http.MethodPatch, "/api/v1/environments/"+environmentID.String(), map[string]any{"auto_deploy": on}).RequireStatus(http.StatusOK)
}

func (f autoFixture) environment(t *testing.T, name string, port int, autoDeploy bool) environments.Environment {
	t.Helper()
	var env environments.Environment
	f.owner.Do(http.MethodPost, "/api/v1/applications/"+f.appID.String()+"/environments", map[string]any{
		"name": name, "server_id": f.server.ServerID, "auto_deploy": autoDeploy,
		"config": map[string]any{"container_port": 80, "published_port": port},
	}).RequireStatus(http.StatusCreated).Decode(&env)
	return env
}

// queueBuild adds a build job; the runner claims builds in creation order.
func (f autoFixture) queueBuild(t *testing.T, trigger string) uuid.UUID {
	t.Helper()
	job, err := f.jobs.Enqueue(t.Context(), jobs.NewJob{ProjectID: f.projectID, ApplicationID: &f.appID,
		Type: agentproto.JobBuild, WorkerKind: jobs.WorkerRunner,
		Payload: agentproto.BuildPayload{ApplicationID: f.appID, GitConnectionID: f.connectionID, RepositoryID: "77",
			RepositoryFullName: "acme/shop", Branch: "main", CommitSHA: commit, Trigger: trigger}})
	if err != nil {
		t.Fatal(err)
	}
	return job.ID
}

func (f autoFixture) claim(t *testing.T) agentproto.Job {
	t.Helper()
	wait := 0
	var job agentproto.Job
	f.runner.Do(http.MethodPost, "/api/v1/runner/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusOK).Decode(&job)
	return job
}

func (f autoFixture) succeed(t *testing.T, job agentproto.Job, ref string) {
	t.Helper()
	result, _ := json.Marshal(agentproto.BuildResult{CommitSHA: commit, ImageRef: ref})
	f.runner.Do(http.MethodPost, "/api/v1/runner/jobs/"+job.ID.String()+"/complete",
		agentproto.CompleteRequest{Attempt: job.Attempt, Outcome: agentproto.OutcomeSucceeded, Result: result}).RequireStatus(http.StatusNoContent)
}

func (f autoFixture) deploymentsOf(t *testing.T, environmentID uuid.UUID) []deployments.Deployment {
	t.Helper()
	var list struct{ Items []deployments.Deployment }
	f.owner.Do(http.MethodGet, "/api/v1/environments/"+environmentID.String()+"/deployments", nil).RequireStatus(http.StatusOK).Decode(&list)
	return list.Items
}

func TestSucceededBuildDeploysToAutoDeployEnvironmentsOnly(t *testing.T) {
	f := newAutoFixture(t)
	f.setAutoDeploy(t, f.staging.ID, true)
	production := f.environment(t, "production", 18081, false)

	build := f.queueBuild(t, agentproto.TriggerPush)
	f.succeed(t, f.claim(t), imageRef)

	staging := f.deploymentsOf(t, f.staging.ID)
	if len(staging) != 1 || staging[0].ImageRef != imageRef || staging[0].Status != "queued" ||
		staging[0].SourceBuildJobID == nil || *staging[0].SourceBuildJobID != build {
		t.Fatalf("staging deployments %+v", staging)
	}
	if got := f.deploymentsOf(t, production.ID); len(got) != 0 {
		t.Fatalf("production deployed without auto_deploy: %+v", got)
	}
	// The deployment job is waiting for the server's Agent.
	wait := 0
	var claimed agentproto.Job
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusOK).Decode(&claimed)
	if claimed.Type != agentproto.JobDeployImage {
		t.Fatalf("claimed %+v", claimed)
	}
}

func TestBuildOfAnExplicitCommitIsNotAutoDeployed(t *testing.T) {
	f := newAutoFixture(t)
	f.setAutoDeploy(t, f.staging.ID, true)
	f.queueBuild(t, agentproto.TriggerCommit)
	f.succeed(t, f.claim(t), imageRef)
	if got := f.deploymentsOf(t, f.staging.ID); len(got) != 0 {
		t.Fatalf("explicit commit build was deployed: %+v", got)
	}
}

func TestOlderBuildFinishingLateDoesNotReplaceNewerDeployment(t *testing.T) {
	f := newAutoFixture(t)
	f.setAutoDeploy(t, f.staging.ID, true)
	f.queueBuild(t, agentproto.TriggerPush)
	f.queueBuild(t, agentproto.TriggerPush)
	older, newer := f.claim(t), f.claim(t)
	newerRef := "ghcr.io/acme/shop-api@sha256:" + "f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0"

	f.succeed(t, newer, newerRef)
	f.succeed(t, older, imageRef)

	got := f.deploymentsOf(t, f.staging.ID)
	if len(got) != 1 || got[0].ImageRef != newerRef {
		t.Fatalf("deployments %+v, want only the newer build", got)
	}
}

func TestOneBrokenEnvironmentDoesNotFailTheBuildOrOthers(t *testing.T) {
	f := newAutoFixture(t)
	broken := f.environment(t, "a-broken", 18082, true)
	healthy := f.environment(t, "b-healthy", 18083, true)
	if _, err := f.env.Pool.Exec(context.Background(), `UPDATE environments SET config = '{"restart":"bogus"}' WHERE id = $1`, broken.ID); err != nil {
		t.Fatal(err)
	}

	build := f.queueBuild(t, agentproto.TriggerPush)
	f.succeed(t, f.claim(t), imageRef)

	var job jobs.Job
	f.owner.Do(http.MethodGet, "/api/v1/jobs/"+build.String(), nil).RequireStatus(http.StatusOK).Decode(&job)
	if job.Status != jobs.StatusSucceeded {
		t.Fatalf("build %+v", job)
	}
	if got := f.deploymentsOf(t, broken.ID); len(got) != 0 {
		t.Fatalf("broken environment deployed: %+v", got)
	}
	if got := f.deploymentsOf(t, healthy.ID); len(got) != 1 {
		t.Fatalf("healthy environment deployments %+v", got)
	}
}
