package agentapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

func complete(agent apitest.Agent, job agentproto.Job, req agentproto.CompleteRequest) *apitest.Response {
	req.Attempt = job.Attempt
	return agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+job.ID.String()+"/complete", req)
}

func claimOne(t *testing.T, f claimFixture) agentproto.Job {
	t.Helper()
	f.enqueue(t, f.agent.ServerID)
	var job agentproto.Job
	claim(f.agent, 0).RequireStatus(http.StatusOK).Decode(&job)
	return job
}

func TestCompleteRecordsOutcomeIdempotently(t *testing.T) {
	f := newClaimFixture(t)
	job := claimOne(t, f)
	done := agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded, Result: json.RawMessage(`{"state":"running"}`)}

	complete(f.agent, job, done).RequireStatus(http.StatusNoContent)
	complete(f.agent, job, done).RequireStatus(http.StatusNoContent)

	var status, attemptStatus string
	var result map[string]string
	err := f.env.Pool.QueryRow(context.Background(), `
		SELECT jobs.status, jobs.result, job_attempts.status
		FROM jobs JOIN job_attempts ON job_attempts.job_id = jobs.id
		WHERE jobs.id = $1`, job.ID).Scan(&status, &result, &attemptStatus)
	if err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || attemptStatus != "succeeded" || result["state"] != "running" {
		t.Fatalf("status=%s attempt=%s result=%v", status, attemptStatus, result)
	}

	failed := agentproto.CompleteRequest{Outcome: agentproto.OutcomeFailed, ErrorCode: "boom"}
	complete(f.agent, job, failed).RequireStatus(http.StatusConflict)
}

func TestCompleteFailureNeedsErrorCode(t *testing.T) {
	f := newClaimFixture(t)
	job := claimOne(t, f)

	complete(f.agent, job, agentproto.CompleteRequest{Outcome: agentproto.OutcomeFailed}).
		RequireStatus(http.StatusUnprocessableEntity)
	complete(f.agent, job, agentproto.CompleteRequest{Outcome: "maybe"}).
		RequireStatus(http.StatusUnprocessableEntity)
	complete(f.agent, job, agentproto.CompleteRequest{
		Outcome: agentproto.OutcomeFailed, ErrorCode: "container_not_found", ErrorMessage: "No such container: api",
	}).RequireStatus(http.StatusNoContent)
}

func TestCompleteRejectsOtherAgent(t *testing.T) {
	f := newClaimFixture(t)
	other := f.env.EnrollAgent(f.owner, f.projectID, "prod-02")
	job := claimOne(t, f)

	complete(other, job, agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded}).
		RequireStatus(http.StatusConflict)
}
