package agentapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

func renewLease(agent apitest.Agent, job agentproto.Job, attempt int32) *apitest.Response {
	return agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+job.ID.String()+"/heartbeat", agentproto.LeaseRequest{Attempt: attempt})
}

func TestRenewLease(t *testing.T) {
	f := newClaimFixture(t)
	f.enqueue(t, f.agent.ServerID)
	var job agentproto.Job
	claim(f.agent, 0).RequireStatus(http.StatusOK).Decode(&job)

	time.Sleep(10 * time.Millisecond)
	var lease agentproto.LeaseResponse
	renewLease(f.agent, job, job.Attempt).RequireStatus(http.StatusOK).Decode(&lease)
	if !lease.LeaseExpiresAt.After(job.LeaseExpiresAt) {
		t.Fatalf("lease not extended: %v -> %v", job.LeaseExpiresAt, lease.LeaseExpiresAt)
	}
}

func TestRenewLeaseRejectsNonOwners(t *testing.T) {
	f := newClaimFixture(t)
	other := f.env.EnrollAgent(f.owner, f.projectID, "prod-02")
	f.enqueue(t, f.agent.ServerID)
	var job agentproto.Job
	claim(f.agent, 0).RequireStatus(http.StatusOK).Decode(&job)

	if code := renewLease(other, job, job.Attempt).RequireStatus(http.StatusConflict).ErrorCode(); code != "lease_lost" {
		t.Fatalf("other agent: code %s", code)
	}
	renewLease(f.agent, job, job.Attempt+1).RequireStatus(http.StatusConflict)

	if _, err := f.env.Pool.Exec(context.Background(), "UPDATE jobs SET status = 'cancelled' WHERE id = $1", job.ID); err != nil {
		t.Fatal(err)
	}
	renewLease(f.agent, job, job.Attempt).RequireStatus(http.StatusConflict)
}
