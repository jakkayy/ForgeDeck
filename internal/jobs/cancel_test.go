package jobs_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

func TestCancelJob(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	viewer, viewerID := env.NewUser("viewer@example.com")
	projectID := owner.CreateProject("Infra")
	env.AddMember(projectID, viewerID, "viewer")
	agent := env.EnrollAgent(owner, projectID, "prod-01")
	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)
	enqueue := func() string {
		job, err := svc.Enqueue(t.Context(), jobs.NewJob{
			ProjectID: projectID, Type: agentproto.JobFetchLogs, WorkerKind: jobs.WorkerAgent,
			TargetServerID: &agent.ServerID, Payload: map[string]string{},
		})
		if err != nil {
			t.Fatal(err)
		}
		return "/api/v1/jobs/" + job.ID.String()
	}

	queued := enqueue()
	viewer.Do(http.MethodPost, queued+"/cancel", nil).RequireStatus(http.StatusForbidden)
	var cancelled jobs.Job
	owner.Do(http.MethodPost, queued+"/cancel", nil).RequireStatus(http.StatusOK).Decode(&cancelled)
	if cancelled.Status != jobs.StatusCancelled || cancelled.FinishedAt == nil {
		t.Fatalf("job %+v", cancelled)
	}
	owner.Do(http.MethodPost, queued+"/cancel", nil).RequireStatus(http.StatusConflict)

	wait := 0
	agent.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusNoContent)

	running := enqueue()
	var claimed agentproto.Job
	agent.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusOK).Decode(&claimed)
	owner.Do(http.MethodPost, running+"/cancel", nil).RequireStatus(http.StatusOK)
	resp := agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+claimed.ID.String()+"/heartbeat", agentproto.LeaseRequest{Attempt: claimed.Attempt})
	if resp.Status != http.StatusConflict || resp.ErrorCode() != "lease_lost" {
		t.Fatalf("renewal after cancel: %d %s", resp.Status, resp.Body)
	}
	var attempt string
	if err := env.Pool.QueryRow(t.Context(), "SELECT status FROM job_attempts WHERE job_id = $1", claimed.ID).Scan(&attempt); err != nil || attempt != "cancelled" {
		t.Fatalf("attempt %q %v", attempt, err)
	}
}
