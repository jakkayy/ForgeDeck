package agentapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
)

type claimFixture struct {
	env       *apitest.Env
	owner     *apitest.Client
	projectID uuid.UUID
	agent     apitest.Agent
	jobs      *jobs.Service
}

func newClaimFixture(t *testing.T) claimFixture {
	t.Helper()
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	return claimFixture{
		env: env, owner: owner, projectID: projectID,
		agent: env.EnrollAgent(owner, projectID, "prod-01"),
		jobs:  jobs.NewService(env.Pool, time.Now),
	}
}

func (f claimFixture) enqueue(t *testing.T, serverID uuid.UUID) uuid.UUID {
	t.Helper()
	job, err := f.jobs.Enqueue(context.Background(), jobs.NewJob{
		ProjectID: f.projectID, Type: "container_action", WorkerKind: jobs.WorkerAgent,
		TargetServerID: &serverID, Payload: map[string]string{"action": "restart"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return job.ID
}

func claim(agent apitest.Agent, waitSeconds int) *apitest.Response {
	return agent.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &waitSeconds})
}

func TestClaimReturnsQueuedJob(t *testing.T) {
	f := newClaimFixture(t)
	jobID := f.enqueue(t, f.agent.ServerID)

	var job agentproto.Job
	claim(f.agent, 0).RequireStatus(http.StatusOK).Decode(&job)
	if job.ID != jobID || job.Type != "container_action" || job.Attempt != 1 || time.Until(job.LeaseExpiresAt) <= 0 {
		t.Fatalf("job %+v", job)
	}
	claim(f.agent, 0).RequireStatus(http.StatusNoContent)
}

func TestClaimIgnoresOtherServersJobs(t *testing.T) {
	f := newClaimFixture(t)
	other := f.env.EnrollAgent(f.owner, f.projectID, "prod-02")
	f.enqueue(t, other.ServerID)

	claim(f.agent, 0).RequireStatus(http.StatusNoContent)
	claim(other, 0).RequireStatus(http.StatusOK)
}

func TestClaimLongPollWakesOnNewJob(t *testing.T) {
	f := newClaimFixture(t)
	result := make(chan *apitest.Response, 1)
	start := time.Now()
	go func() { result <- claim(f.agent, 10) }()

	time.Sleep(300 * time.Millisecond)
	jobID := f.enqueue(t, f.agent.ServerID)

	select {
	case resp := <-result:
		var job agentproto.Job
		resp.RequireStatus(http.StatusOK).Decode(&job)
		if job.ID != jobID {
			t.Fatalf("got job %s, want %s", job.ID, jobID)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Fatalf("long-poll took %v; notification did not wake it", elapsed)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("long-poll did not return after a job was queued")
	}
}

func TestClaimLongPollEndsWhenServerIsRemoved(t *testing.T) {
	f := newClaimFixture(t)
	result := make(chan *apitest.Response, 1)
	go func() { result <- claim(f.agent, 20) }()

	time.Sleep(300 * time.Millisecond)
	f.owner.Do(http.MethodDelete, "/api/v1/servers/"+f.agent.ServerID.String(), nil).RequireStatus(http.StatusNoContent)

	select {
	case resp := <-result:
		resp.RequireStatus(http.StatusUnauthorized)
	case <-time.After(5 * time.Second):
		t.Fatal("pending long-poll was not released after the server was removed")
	}
}

func TestClaimRejectsInvalidWait(t *testing.T) {
	f := newClaimFixture(t)
	claim(f.agent, agentproto.MaxClaimWaitSeconds+1).RequireStatus(http.StatusBadRequest)
}
