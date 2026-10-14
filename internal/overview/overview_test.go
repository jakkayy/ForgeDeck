package overview_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/overview"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

const commit = "1111111111111111111111111111111111111111"

// digestOf expands a two-digit seed into a full sha256 digest.
func digestOf(seed string) string { return "sha256:" + strings.Repeat(seed, 32) }

type fixture struct {
	env       *apitest.Env
	owner     *apitest.Client
	projectID uuid.UUID
	appID     uuid.UUID
	server    apitest.Agent
	jobs      *jobs.Service
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	return fixture{
		env: env, owner: owner, projectID: projectID,
		appID:  env.CreateApplication(owner, projectID, "API"),
		server: env.EnrollAgent(owner, projectID, "prod-01"),
		jobs:   jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now),
	}
}

func (f fixture) environment(t *testing.T, name string, port int) uuid.UUID {
	t.Helper()
	var created struct{ ID uuid.UUID }
	f.owner.Do(http.MethodPost, "/api/v1/applications/"+f.appID.String()+"/environments", map[string]any{
		"name": name, "server_id": f.server.ServerID,
		"config": map[string]any{"container_port": 8080, "published_port": port},
	}).RequireStatus(http.StatusCreated).Decode(&created)
	return created.ID
}

// deploy queues a deployment to the environment, has the Agent claim it, and
// finishes it with the given outcome.
func (f fixture) deploy(t *testing.T, environmentID uuid.UUID, seed, outcome string) {
	t.Helper()
	job, err := f.jobs.Enqueue(t.Context(), jobs.NewJob{ProjectID: f.projectID, ApplicationID: &f.appID,
		Type: agentproto.JobBuild, WorkerKind: jobs.WorkerRunner, Payload: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	result, _ := json.Marshal(agentproto.BuildResult{CommitSHA: commit, Image: "ghcr.io/acme/shop-api", Tag: commit[:12],
		Digest: digestOf(seed), ImageRef: "ghcr.io/acme/shop-api@" + digestOf(seed)})
	if _, err := f.env.Pool.Exec(context.Background(), `UPDATE jobs SET status = 'succeeded', result = $2 WHERE id = $1`, job.ID, result); err != nil {
		t.Fatal(err)
	}
	f.owner.Do(http.MethodPost, "/api/v1/environments/"+environmentID.String()+"/deployments",
		map[string]any{"build_job_id": job.ID}).RequireStatus(http.StatusAccepted)

	wait := 0
	var claimed agentproto.Job
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).
		RequireStatus(http.StatusOK).Decode(&claimed)
	req := agentproto.CompleteRequest{Attempt: claimed.Attempt, Outcome: outcome}
	if outcome == agentproto.OutcomeFailed {
		req.ErrorCode, req.ErrorMessage = "health_check_failed", "did not become healthy"
	}
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/"+claimed.ID.String()+"/complete", req).RequireStatus(http.StatusNoContent)
}

func get(t *testing.T, client *apitest.Client) overview.Overview {
	t.Helper()
	var o overview.Overview
	client.Do(http.MethodGet, "/api/v1/overview", nil).RequireStatus(http.StatusOK).Decode(&o)
	return o
}

func TestOverviewSummarizesServersAndDeployments(t *testing.T) {
	f := newFixture(t)
	f.env.EnrollAgent(f.owner, f.projectID, "prod-02") // never reports in: pending
	f.server.Do(http.MethodPost, "/api/v1/agent/heartbeat", agentproto.HeartbeatRequest{
		AgentVersion: "0.2.0", ProtocolVersion: agentproto.ProtocolVersion, Docker: agentproto.Docker{Available: true},
	}).RequireStatus(http.StatusOK)
	staging := f.environment(t, "staging", 18080)
	production := f.environment(t, "production", 18081)
	f.environment(t, "preview", 18082) // never deployed

	f.deploy(t, production, "aa", agentproto.OutcomeSucceeded)
	f.deploy(t, staging, "bb", agentproto.OutcomeFailed)

	o := get(t, f.owner)
	if o.Servers != (overview.ServerCounts{Total: 2, Online: 1, Pending: 1}) {
		t.Fatalf("servers %+v", o.Servers)
	}
	if o.Environments != (overview.EnvCounts{Total: 3, Succeeded: 1, Failed: 1, NotDeployed: 1}) {
		t.Fatalf("environments %+v", o.Environments)
	}
	if len(o.Recent) != 2 || o.Recent[0].EnvironmentName != "staging" || o.Recent[0].Status != "failed" ||
		o.Recent[0].ApplicationName != "API" || o.Recent[0].ProjectName != "Shop" || o.Recent[1].Status != "succeeded" {
		t.Fatalf("recent %+v", o.Recent)
	}
	if len(o.Failures) != 1 || o.Failures[0].EnvironmentID != staging || o.Failures[0].ErrorCode == nil || *o.Failures[0].ErrorCode != "health_check_failed" {
		t.Fatalf("failures %+v", o.Failures)
	}

	// A later success clears the failure: only what still needs action is listed.
	f.deploy(t, staging, "cc", agentproto.OutcomeSucceeded)
	o = get(t, f.owner)
	if len(o.Failures) != 0 || o.Environments.Failed != 0 || o.Environments.Succeeded != 2 || len(o.Recent) != 3 {
		t.Fatalf("after recovery: %+v", o)
	}
}

func TestOverviewShowsOnlyTheCallersProjects(t *testing.T) {
	f := newFixture(t)
	f.deploy(t, f.environment(t, "staging", 18080), "aa", agentproto.OutcomeFailed)
	outsider, _ := f.env.NewUser("outsider@example.com")

	o := get(t, outsider)
	if o.Servers.Total != 0 || o.Environments.Total != 0 || len(o.Recent) != 0 || len(o.Failures) != 0 {
		t.Fatalf("an outsider sees %+v", o)
	}
	f.env.Anonymous().Do(http.MethodGet, "/api/v1/overview", nil).RequireStatus(http.StatusUnauthorized)
}

func TestOverviewClassifiesServersByHeartbeatAge(t *testing.T) {
	f := newFixture(t)
	for name, age := range map[string]time.Duration{"fresh": time.Second, "stale": 3 * time.Minute, "gone": time.Hour} {
		agent := f.env.EnrollAgent(f.owner, f.projectID, name)
		if _, err := f.env.Pool.Exec(context.Background(), `UPDATE servers SET last_seen_at = $2 WHERE id = $1`,
			agent.ServerID, time.Now().Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	if got := get(t, f.owner).Servers; got != (overview.ServerCounts{Total: 4, Online: 1, Unreachable: 1, Offline: 1, Pending: 1}) {
		t.Fatalf("servers %+v", got)
	}
}
