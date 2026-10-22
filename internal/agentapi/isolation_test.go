package agentapi_test

import (
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

// TestAgentsOnlyReceiveTheirOwnServersJobs checks jobs reach only the Agent of their server.
func TestAgentsOnlyReceiveTheirOwnServersJobs(t *testing.T) {
	f := newClaimFixture(t)
	otherOwner, _ := f.env.NewUser("other@example.com")
	otherProject := otherOwner.CreateProject("Other team")
	agents := []apitest.Agent{
		f.agent,
		f.env.EnrollAgent(f.owner, f.projectID, "prod-02"),
		f.env.EnrollAgent(otherOwner, otherProject, "their-prod"),
	}

	target := map[uuid.UUID]uuid.UUID{}
	for _, agent := range agents {
		for range 5 {
			target[enqueueFor(t, f, agent)] = agent.ServerID
		}
	}

	var (
		mu        sync.Mutex
		claimedBy = map[uuid.UUID]uuid.UUID{}
		wg        sync.WaitGroup
	)
	for _, agent := range agents {
		wg.Go(func() {
			for {
				resp := claim(agent, 0)
				if resp.Status == http.StatusNoContent {
					return
				}
				var job agentproto.Job
				resp.RequireStatus(http.StatusOK).Decode(&job)
				mu.Lock()
				claimedBy[job.ID] = agent.ServerID
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if len(claimedBy) != len(target) {
		t.Fatalf("claimed %d of %d jobs", len(claimedBy), len(target))
	}
	for jobID, server := range target {
		if claimedBy[jobID] != server {
			t.Errorf("job for server %s was claimed by the Agent of %s", server, claimedBy[jobID])
		}
	}
}

func enqueueFor(t *testing.T, f claimFixture, agent apitest.Agent) uuid.UUID {
	t.Helper()
	var projectID uuid.UUID
	if err := f.env.Pool.QueryRow(t.Context(), "SELECT project_id FROM servers WHERE id = $1", agent.ServerID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	fixture := f
	fixture.projectID = projectID
	return fixture.enqueue(t, agent.ServerID)
}
