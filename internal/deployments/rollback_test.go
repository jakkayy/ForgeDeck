package deployments_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/deployments"
	"github.com/jakkayy/ForgeDeck/internal/environments"
)

func (f fixture) rollback(client *apitest.Client, releaseID uuid.UUID) *apitest.Response {
	return client.Do(http.MethodPost, "/api/v1/releases/"+releaseID.String()+"/rollback", nil)
}

// twoReleases deploys A then B healthily and returns their IDs (A first).
func (f fixture) twoReleases(t *testing.T) (a, b deployments.Release) {
	t.Helper()
	for _, digest := range []string{digestA, digestB} {
		_, job := f.deployAndClaim(t, refOf(digest))
		f.agentComplete(t, job, agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded})
	}
	got := f.releases(t)
	return got[1], got[0]
}

func TestRollbackRedeploysTheOldDigestAndConfiguration(t *testing.T) {
	f := newFixture(t)
	a, b := f.twoReleases(t)
	// The environment's configuration changed after release A.
	f.owner.Do(http.MethodPatch, "/api/v1/environments/"+f.staging.ID.String(), map[string]any{
		"config": map[string]any{"container_port": 8080, "published_port": 18080, "env": map[string]string{"LOG_LEVEL": "debug"}},
	}).RequireStatus(http.StatusOK)

	var d deployments.Deployment
	f.rollback(f.owner, a.ID).RequireStatus(http.StatusAccepted).Decode(&d)
	if d.SourceReleaseID == nil || *d.SourceReleaseID != a.ID || d.SourceBuildJobID != nil || d.ImageRef != refOf(digestA) || d.Status != "queued" {
		t.Fatalf("deployment %+v", d)
	}

	// The job carries release A's digest and A's own configuration (info, not debug).
	wait := 0
	var job agentproto.Job
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusOK).Decode(&job)
	var payload agentproto.DeployImagePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ImageRef != refOf(digestA) || payload.Config.Env["LOG_LEVEL"] != "info" || payload.CommitSHA != a.CommitSHA {
		t.Fatalf("payload %+v", payload)
	}
	// The previous release it would fall back to is B, the one running now.
	if job.Deploy == nil || job.Deploy.Previous == nil || job.Deploy.Previous.ReleaseID != b.ID {
		t.Fatalf("deploy context %+v", job.Deploy)
	}

	// Once healthy it is the newest release; history keeps all three.
	f.agentComplete(t, job, agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded})
	got := f.releases(t)
	if len(got) != 3 || got[0].ImageRef != refOf(digestA) || !got[0].Current || got[1].ID != b.ID || got[1].Current || got[2].ID != a.ID {
		t.Fatalf("releases %+v", got)
	}
	// Rolling back never rewrites the environment's own configuration.
	var env environments.Environment
	f.owner.Do(http.MethodGet, "/api/v1/environments/"+f.staging.ID.String(), nil).RequireStatus(http.StatusOK).Decode(&env)
	if env.Config.Env["LOG_LEVEL"] != "debug" {
		t.Fatalf("environment config %+v", env.Config)
	}
}

func TestRollbackRefusesAPortAnotherEnvironmentNowUses(t *testing.T) {
	f := newFixture(t)
	a, _ := f.twoReleases(t)
	// Move this environment off port 18080 and let another environment take it.
	f.owner.Do(http.MethodPatch, "/api/v1/environments/"+f.staging.ID.String(), map[string]any{
		"config": map[string]any{"container_port": 8080, "published_port": 18090},
	}).RequireStatus(http.StatusOK)
	f.owner.Do(http.MethodPost, "/api/v1/applications/"+f.appID.String()+"/environments", map[string]any{
		"name": "production", "server_id": f.server.ServerID,
		"config": map[string]any{"container_port": 8080, "published_port": 18080},
	}).RequireStatus(http.StatusCreated)

	if code := f.rollback(f.owner, a.ID).RequireStatus(http.StatusConflict).ErrorCode(); code != "port_in_use" {
		t.Fatalf("code %s", code)
	}
}

func TestRollbackAuthorization(t *testing.T) {
	f := newFixture(t)
	a, _ := f.twoReleases(t)
	viewer, viewerID := f.env.NewUser("viewer@example.com")
	f.env.AddMember(f.projectID, viewerID, "viewer")
	developer, developerID := f.env.NewUser("dev@example.com")
	f.env.AddMember(f.projectID, developerID, "developer")
	outsider, _ := f.env.NewUser("outsider@example.com")

	f.rollback(viewer, a.ID).RequireStatus(http.StatusForbidden)
	f.rollback(outsider, a.ID).RequireStatus(http.StatusNotFound)
	f.rollback(f.owner, uuid.New()).RequireStatus(http.StatusNotFound)
	f.rollback(developer, a.ID).RequireStatus(http.StatusAccepted)
}
