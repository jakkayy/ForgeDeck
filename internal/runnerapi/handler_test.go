package runnerapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

const echoJob = "echo"

func TestRunnerClaimsOnlyRunnerJobsAndCompletesThem(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	agent := env.EnrollAgent(owner, projectID, "prod-01")
	runner := env.NewRunner("build-01")
	other := env.NewRunner("build-02")
	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)
	enqueue := func(kind jobs.WorkerKind, target *uuid.UUID) uuid.UUID {
		job, err := svc.Enqueue(t.Context(), jobs.NewJob{ProjectID: projectID, Type: echoJob,
			WorkerKind: kind, TargetServerID: target, Payload: map[string]string{}})
		if err != nil {
			t.Fatal(err)
		}
		return job.ID
	}
	enqueue(jobs.WorkerAgent, &agent.ServerID)
	buildID := enqueue(jobs.WorkerRunner, nil)
	wait := 0

	runner.Do(http.MethodPost, "/api/v1/runner/heartbeat", agentproto.HeartbeatRequest{AgentVersion: "0.1", ProtocolVersion: agentproto.ProtocolVersion}).
		RequireStatus(http.StatusOK)
	var job agentproto.Job
	runner.Do(http.MethodPost, "/api/v1/runner/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusOK).Decode(&job)
	if job.ID != buildID {
		t.Fatalf("runner claimed %s, want the runner job", job.ID)
	}
	runner.Do(http.MethodPost, "/api/v1/runner/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusNoContent)

	base := "/api/v1/runner/jobs/" + job.ID.String()
	if resp := other.Do(http.MethodPost, base+"/heartbeat", agentproto.LeaseRequest{Attempt: job.Attempt}); resp.Status != http.StatusConflict {
		t.Fatalf("other runner renewed: %d", resp.Status)
	}
	if resp := agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+job.ID.String()+"/complete",
		agentproto.CompleteRequest{Attempt: job.Attempt, Outcome: agentproto.OutcomeSucceeded}); resp.Status != http.StatusConflict {
		t.Fatalf("agent completed a runner job: %d", resp.Status)
	}

	runner.Do(http.MethodPost, base+"/heartbeat", agentproto.LeaseRequest{Attempt: job.Attempt}).RequireStatus(http.StatusOK)
	runner.Do(http.MethodPost, base+"/events", agentproto.EventsRequest{Attempt: job.Attempt,
		Events: []agentproto.LogEvent{{Sequence: 1, Stream: "stdout", Message: "building"}}}).RequireStatus(http.StatusNoContent)
	runner.Do(http.MethodPost, base+"/complete", agentproto.CompleteRequest{Attempt: job.Attempt, Outcome: agentproto.OutcomeSucceeded}).
		RequireStatus(http.StatusNoContent)
}

func TestRunnerEndpointsRequireCredential(t *testing.T) {
	env := apitest.New(t)
	wait := 0
	env.Anonymous().Do(http.MethodPost, "/api/v1/runner/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).
		RequireStatus(http.StatusUnauthorized)
	owner, _ := env.NewUser("owner@example.com")
	agent := env.EnrollAgent(owner, owner.CreateProject("Shop"), "prod-01")
	agent.Do(http.MethodPost, "/api/v1/runner/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).
		RequireStatus(http.StatusUnauthorized)
}

func TestRunnerLongPollWakesOnQueuedJob(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	runner := env.NewRunner("build-01")
	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)

	result := make(chan *apitest.Response, 1)
	go func() {
		wait := 10
		result <- runner.Do(http.MethodPost, "/api/v1/runner/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait})
	}()
	time.Sleep(300 * time.Millisecond)
	if _, err := svc.Enqueue(t.Context(), jobs.NewJob{ProjectID: projectID, Type: echoJob,
		WorkerKind: jobs.WorkerRunner, Payload: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	select {
	case resp := <-result:
		resp.RequireStatus(http.StatusOK)
	case <-time.After(5 * time.Second):
		t.Fatal("runner long-poll was not woken by a queued job")
	}
}
