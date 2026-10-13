package deployments_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/audit"
	"github.com/jakkayy/ForgeDeck/internal/deployments"
	"github.com/jakkayy/ForgeDeck/internal/environments"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
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

func TestClaimedDeploymentCarriesScopedRegistryCredentialsAndPreviousRelease(t *testing.T) {
	f := newAutoFixture(t)
	// A second registry of the project must never be sent for this image.
	f.owner.Do(http.MethodPost, "/api/v1/projects/"+f.projectID.String()+"/registries",
		map[string]string{"name": "Other", "host": "registry.other.example", "username": "other", "password": "other-secret"}).
		RequireStatus(http.StatusCreated)
	// The environment already runs a release.
	releaseID := f.recordRelease(t, f.staging.ID, "ghcr.io/acme/shop-api@sha256:"+"a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1")

	build := f.build(t, f.appID, "succeeded", goodResult())
	var d deployments.Deployment
	f.deploy(f.owner, f.staging.ID, build).RequireStatus(http.StatusAccepted).Decode(&d)

	claimed := f.claimDeploy(t)
	want := agentproto.RegistryCredentials{Host: "ghcr.io", Username: "bot", Password: "secret"}
	if claimed.Deploy == nil || claimed.Deploy.Registry == nil || *claimed.Deploy.Registry != want {
		t.Fatalf("registry credentials %+v", claimed.Deploy)
	}
	if prev := claimed.Deploy.Previous; prev == nil || prev.ReleaseID != releaseID || *prev.Config.PublishedPort != 18080 {
		t.Fatalf("previous release %+v", claimed.Deploy.Previous)
	}

	// Secrets travel only in the claim response, not in the stored job.
	var stored jobs.Job
	f.owner.Do(http.MethodGet, "/api/v1/jobs/"+d.JobID.String(), nil).RequireStatus(http.StatusOK).Decode(&stored)
	// (The registry password in this fixture is "secret".)
	if strings.Contains(string(stored.Payload), `"password"`) || strings.Contains(string(stored.Payload), `":"secret"`) {
		t.Fatalf("payload %s", stored.Payload)
	}
}

func TestDeploymentOfAnImageFromAnUnknownRegistryHasNoCredentials(t *testing.T) {
	f := newAutoFixture(t)
	result := goodResult()
	result.ImageRef = "unknown.example/acme/shop-api@" + digest
	build := f.build(t, f.appID, "succeeded", result)
	f.deploy(f.owner, f.staging.ID, build).RequireStatus(http.StatusAccepted)

	claimed := f.claimDeploy(t)
	if claimed.Deploy == nil || claimed.Deploy.Registry != nil || claimed.Deploy.Previous != nil {
		t.Fatalf("deploy context %+v", claimed.Deploy)
	}
}

func TestDeploymentWhoseEnvironmentWasDeletedFailsAtClaim(t *testing.T) {
	f := newAutoFixture(t)
	build := f.build(t, f.appID, "succeeded", goodResult())
	var d deployments.Deployment
	f.deploy(f.owner, f.staging.ID, build).RequireStatus(http.StatusAccepted).Decode(&d)
	f.owner.Do(http.MethodDelete, "/api/v1/environments/"+f.staging.ID.String(), nil).RequireStatus(http.StatusNoContent)

	wait := 0
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusNoContent)
	var job jobs.Job
	f.owner.Do(http.MethodGet, "/api/v1/jobs/"+d.JobID.String(), nil).RequireStatus(http.StatusOK).Decode(&job)
	if job.Status != jobs.StatusFailed || job.Error == nil || job.Error.Code != "environment_not_found" {
		t.Fatalf("job %+v", job)
	}
}

func (f autoFixture) claimDeploy(t *testing.T) agentproto.Job {
	t.Helper()
	wait := 0
	var job agentproto.Job
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusOK).Decode(&job)
	if job.Type != agentproto.JobDeployImage {
		t.Fatalf("claimed %+v", job)
	}
	return job
}

// recordRelease inserts a release row for the environment, as a healthy
// deployment will once release recording exists.
func (f autoFixture) recordRelease(t *testing.T, environmentID uuid.UUID, ref string) uuid.UUID {
	t.Helper()
	config := `{"container_port":80,"published_port":18080,"bind_address":"127.0.0.1","env":{},"volumes":[],"restart":"unless-stopped","health_check":{"type":"tcp","timeout_seconds":60,"interval_seconds":2}}`
	ctx := context.Background()
	var deploymentID, releaseID uuid.UUID
	err := f.env.Pool.QueryRow(ctx, `INSERT INTO deployments (environment_id, commit_sha, image_ref, config, status, source_release_id)
		VALUES ($1, $2, $3, $4::jsonb, 'succeeded', NULL) RETURNING id`, environmentID, commit, ref, config).Scan(&deploymentID)
	if err != nil {
		t.Fatal(err)
	}
	err = f.env.Pool.QueryRow(ctx, `INSERT INTO releases (application_id, environment_id, deployment_id, commit_sha, image_ref, config)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb) RETURNING id`, f.appID, environmentID, deploymentID, commit, ref, config).Scan(&releaseID)
	if err != nil {
		t.Fatal(err)
	}
	return releaseID
}

func (f autoFixture) setSecret(t *testing.T, name, value string) {
	t.Helper()
	f.owner.Do(http.MethodPut, "/api/v1/projects/"+f.projectID.String()+"/secrets/"+name, map[string]string{"value": value}).
		RequireStatus(http.StatusCreated)
}

func (f autoFixture) useSecrets(t *testing.T, names ...string) {
	t.Helper()
	f.owner.Do(http.MethodPatch, "/api/v1/environments/"+f.staging.ID.String(), map[string]any{
		"config": map[string]any{"container_port": 8080, "published_port": 18080, "secrets": names},
	}).RequireStatus(http.StatusOK)
}

func TestClaimedDeploymentCarriesCurrentSecretValuesOnly(t *testing.T) {
	f := newAutoFixture(t)
	f.setSecret(t, "DATABASE_PASSWORD", "first-database-value")
	f.setSecret(t, "OTHER_PROJECT_SECRET", "unrelated-secret-value")
	f.useSecrets(t, "DATABASE_PASSWORD")

	build := f.build(t, f.appID, "succeeded", goodResult())
	var d deployments.Deployment
	f.deploy(f.owner, f.staging.ID, build).RequireStatus(http.StatusAccepted).Decode(&d)
	// The secret is rotated after the deployment was queued: the claim has the new value.
	f.setSecretReplace(t, "DATABASE_PASSWORD", "rotated-database-value")

	claimed := f.claimDeploy(t)
	if got := claimed.Deploy.Secrets; len(got) != 1 || got["DATABASE_PASSWORD"] != "rotated-database-value" {
		t.Fatalf("secrets %v", got)
	}
	// Values are never stored: not in the job, the deployment, or a later release's snapshot.
	var stored jobs.Job
	f.owner.Do(http.MethodGet, "/api/v1/jobs/"+d.JobID.String(), nil).RequireStatus(http.StatusOK).Decode(&stored)
	f.agentComplete(t, claimed, agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded})
	for name, body := range map[string]string{
		"job":        string(stored.Payload),
		"deployment": string(f.owner.Do(http.MethodGet, "/api/v1/deployments/"+d.ID.String(), nil).Body),
		"releases":   string(f.owner.Do(http.MethodGet, "/api/v1/environments/"+f.staging.ID.String()+"/releases", nil).Body),
	} {
		if strings.Contains(body, "database-value") {
			t.Fatalf("a secret value is in the %s: %s", name, body)
		}
	}
	if releases := f.releases(t); len(releases) != 1 || len(releases[0].Config.Secrets) != 1 || releases[0].Config.Secrets[0] != "DATABASE_PASSWORD" {
		t.Fatalf("release snapshot %+v", releases)
	}
}

func (f autoFixture) setSecretReplace(t *testing.T, name, value string) {
	t.Helper()
	f.owner.Do(http.MethodPut, "/api/v1/projects/"+f.projectID.String()+"/secrets/"+name, map[string]string{"value": value}).
		RequireStatus(http.StatusOK)
}

func TestRollbackUsesTheCurrentSecretValue(t *testing.T) {
	f := newAutoFixture(t)
	f.setSecret(t, "API_TOKEN", "token-before-rotation")
	f.useSecrets(t, "API_TOKEN")
	a, _ := f.twoReleases(t)
	f.setSecretReplace(t, "API_TOKEN", "token-after-rotation")

	f.rollback(f.owner, a.ID).RequireStatus(http.StatusAccepted)
	claimed := f.claimDeploy(t)
	if got := claimed.Deploy.Secrets["API_TOKEN"]; got != "token-after-rotation" {
		t.Fatalf("rollback delivered %q: a rollback must not bring back a rotated secret", got)
	}
}

func TestDeploymentFailsAtClaimWhenAListedSecretIsGone(t *testing.T) {
	f := newAutoFixture(t)
	f.setSecret(t, "DATABASE_PASSWORD", "a-long-enough-value")
	f.useSecrets(t, "DATABASE_PASSWORD")
	build := f.build(t, f.appID, "succeeded", goodResult())
	var d deployments.Deployment
	f.deploy(f.owner, f.staging.ID, build).RequireStatus(http.StatusAccepted).Decode(&d)
	// Deleting it is refused while listed, so remove it behind the API's back.
	if _, err := f.env.Pool.Exec(context.Background(), `DELETE FROM project_secrets`); err != nil {
		t.Fatal(err)
	}

	wait := 0
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusNoContent)
	var job jobs.Job
	f.owner.Do(http.MethodGet, "/api/v1/jobs/"+d.JobID.String(), nil).RequireStatus(http.StatusOK).Decode(&job)
	if job.Status != jobs.StatusFailed || job.Error == nil || job.Error.Code != "secret_not_found" || !strings.Contains(job.Error.Message, "DATABASE_PASSWORD") {
		t.Fatalf("job %+v", job)
	}
}

// auditEvents returns the project's audit entries of one action, newest first.
func (f fixture) auditEvents(t *testing.T, action string) []audit.Entry {
	t.Helper()
	var page httpx.List[audit.Entry]
	f.owner.Do(http.MethodGet, "/api/v1/projects/"+f.projectID.String()+"/audit-logs?limit=100&action="+action, nil).
		RequireStatus(http.StatusOK).Decode(&page)
	return page.Items
}

func TestDeploymentsAreAudited(t *testing.T) {
	f := newAutoFixture(t)
	f.setAutoDeploy(t, f.staging.ID, true)

	// An automatic deployment is the system's, tied to the build that caused it.
	f.queueBuild(t, agentproto.TriggerPush)
	f.succeed(t, f.claim(t), imageRef)
	auto := f.auditEvents(t, "deployment.start")
	if len(auto) != 1 || auto[0].Actor != "system" || auto[0].TargetName == nil || *auto[0].TargetName != "staging" {
		t.Fatalf("auto-deploy events %+v", auto)
	}
	var metadata map[string]any
	_ = json.Unmarshal(auto[0].Metadata, &metadata)
	if metadata["trigger"] != "auto_deploy" || metadata["deployment_id"] == nil {
		t.Fatalf("metadata %v", metadata)
	}

	// A manual deployment and a rollback are the user's.
	build := f.build(t, f.appID, "succeeded", goodResult())
	f.deploy(f.owner, f.staging.ID, build).RequireStatus(http.StatusAccepted)
	manual := f.auditEvents(t, "deployment.start")
	if len(manual) != 2 || manual[0].Actor != "owner@example.com" {
		t.Fatalf("manual deployment events %+v", manual)
	}
	releaseID := f.recordRelease(t, f.staging.ID, refOf(digestA))
	f.rollback(f.owner, releaseID).RequireStatus(http.StatusAccepted)
	if rollbacks := f.auditEvents(t, "release.rollback"); len(rollbacks) != 1 || rollbacks[0].Actor != "owner@example.com" {
		t.Fatalf("rollback events %+v", rollbacks)
	}
}
