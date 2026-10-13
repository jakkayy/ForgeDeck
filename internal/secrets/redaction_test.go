package secrets_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// runJob has the project's Agent claim a new job and send the given lines.
func runJob(t *testing.T, env *apitest.Env, owner *apitest.Client, agent apitest.Agent, projectID uuid.UUID, lines ...string) []jobs.LogLine {
	t.Helper()
	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)
	queued, err := svc.Enqueue(t.Context(), jobs.NewJob{ProjectID: projectID, Type: "echo", WorkerKind: jobs.WorkerAgent,
		TargetServerID: &agent.ServerID, Payload: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	wait := 0
	var job agentproto.Job
	agent.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusOK).Decode(&job)

	events := make([]agentproto.LogEvent, len(lines))
	for i, line := range lines {
		events[i] = agentproto.LogEvent{Sequence: int64(i + 1), Stream: "stdout", Message: line}
	}
	agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+job.ID.String()+"/events", agentproto.EventsRequest{Attempt: job.Attempt, Events: events}).
		RequireStatus(http.StatusNoContent)

	var page httpx.List[jobs.LogLine]
	owner.Do(http.MethodGet, "/api/v1/jobs/"+queued.ID.String()+"/logs", nil).RequireStatus(http.StatusOK).Decode(&page)
	return page.Items
}

func TestProjectSecretsAreMaskedInStoredLogs(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	agent := env.EnrollAgent(owner, projectID, "prod-01")
	put(owner, projectID, "DATABASE_PASSWORD", "db-secret-value-1").RequireStatus(http.StatusCreated)

	// Even if the Agent forgot to mask a value, it is masked before it is stored.
	logs := runJob(t, env, owner, agent, projectID,
		"connecting with db-secret-value-1", "nothing to hide here", "db-secret-value-1 twice db-secret-value-1")
	if len(logs) != 3 || logs[0].Message != "connecting with ********" || logs[1].Message != "nothing to hide here" ||
		logs[2].Message != "******** twice ********" {
		t.Fatalf("logs %+v", logs)
	}

	// A replaced secret is masked at once, and the old value is no longer special.
	put(owner, projectID, "DATABASE_PASSWORD", "db-secret-value-2").RequireStatus(http.StatusOK)
	logs = runJob(t, env, owner, agent, projectID, "old db-secret-value-1 new db-secret-value-2")
	if len(logs) != 1 || logs[0].Message != "old db-secret-value-1 new ********" {
		t.Fatalf("logs after replacing %+v", logs)
	}

	// A deleted secret is no longer masked.
	owner.Do(http.MethodDelete, "/api/v1/projects/"+projectID.String()+"/secrets/DATABASE_PASSWORD", nil).RequireStatus(http.StatusNoContent)
	logs = runJob(t, env, owner, agent, projectID, "db-secret-value-2")
	if len(logs) != 1 || !strings.Contains(logs[0].Message, "db-secret-value-2") {
		t.Fatalf("logs after deleting %+v", logs)
	}
}

func TestOtherProjectsSecretsAreNotMasked(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	mine := owner.CreateProject("Mine")
	theirs := owner.CreateProject("Theirs")
	agent := env.EnrollAgent(owner, mine, "prod-01")
	put(owner, theirs, "TOKEN", "their-token-value").RequireStatus(http.StatusCreated)

	logs := runJob(t, env, owner, agent, mine, "their-token-value")
	if len(logs) != 1 || logs[0].Message != "their-token-value" {
		t.Fatalf("logs %+v", logs)
	}
}
