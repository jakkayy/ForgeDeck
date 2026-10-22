package agentapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

// TestRevokedAgentIsLockedOutEverywhere checks every Agent endpoint rejects a revoked credential.
func TestRevokedAgentIsLockedOutEverywhere(t *testing.T) {
	f := newClaimFixture(t)
	f.enqueue(t, f.agent.ServerID)
	var job agentproto.Job
	claim(f.agent, 0).RequireStatus(http.StatusOK).Decode(&job)

	pending := make(chan *apitest.Response, 1)
	go func() { pending <- claim(f.agent, 20) }()
	time.Sleep(300 * time.Millisecond)

	f.owner.Do(http.MethodDelete, "/api/v1/servers/"+f.agent.ServerID.String(), nil).RequireStatus(http.StatusNoContent)

	select {
	case resp := <-pending:
		resp.RequireStatus(http.StatusUnauthorized)
	case <-time.After(5 * time.Second):
		t.Fatal("pending long-poll was not released")
	}

	jobPath := "/api/v1/agent/jobs/" + job.ID.String()
	requests := map[string]any{
		"/api/v1/agent/heartbeat":           agentproto.HeartbeatRequest{AgentVersion: "x", ProtocolVersion: agentproto.ProtocolVersion},
		"/api/v1/agent/containers/snapshot": agentproto.ContainerSnapshot{ObservedAt: time.Now()},
		"/api/v1/agent/jobs/claim":          agentproto.ClaimRequest{},
		jobPath + "/heartbeat":              agentproto.LeaseRequest{Attempt: job.Attempt},
		jobPath + "/events":                 agentproto.EventsRequest{Attempt: job.Attempt},
		jobPath + "/complete":               agentproto.CompleteRequest{Attempt: job.Attempt, Outcome: agentproto.OutcomeSucceeded},
	}
	for path, body := range requests {
		if resp := f.agent.Do(http.MethodPost, path, body); resp.Status != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", path, resp.Status)
		}
	}
}
