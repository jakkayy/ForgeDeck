package jobs_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

func TestJobDetailAndLogs(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	outsider, _ := env.NewUser("outsider@example.com")
	projectID := owner.CreateProject("Infra")
	agent := env.EnrollAgent(owner, projectID, "prod-01")

	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)
	queued, err := svc.Enqueue(context.Background(), jobs.NewJob{
		ProjectID: projectID, Type: "container_action", WorkerKind: jobs.WorkerAgent,
		TargetServerID: &agent.ServerID, Payload: map[string]string{"action": "restart"},
	})
	if err != nil {
		t.Fatal(err)
	}

	wait := 0
	var claimed agentproto.Job
	agent.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).
		RequireStatus(http.StatusOK).Decode(&claimed)
	events := []agentproto.LogEvent{
		{Sequence: 1, Stream: "system", Message: "restarting api"},
		{Sequence: 2, Stream: "stderr", Message: "No such container: api"},
		{Sequence: 3, Stream: "system", Message: "giving up"},
	}
	agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+claimed.ID.String()+"/events",
		agentproto.EventsRequest{Attempt: claimed.Attempt, Events: events}).RequireStatus(http.StatusNoContent)
	agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+claimed.ID.String()+"/complete", agentproto.CompleteRequest{
		Attempt: claimed.Attempt, Outcome: agentproto.OutcomeFailed,
		ErrorCode: "container_not_found", ErrorMessage: "Container api no longer exists.",
	}).RequireStatus(http.StatusNoContent)

	path := "/api/v1/jobs/" + queued.ID.String()
	var job jobs.Job
	owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK).Decode(&job)
	if job.Status != jobs.StatusFailed || job.Error == nil || job.Error.Code != "container_not_found" ||
		job.StartedAt == nil || job.FinishedAt == nil || job.AttemptCount != 1 {
		t.Fatalf("job %+v", job)
	}

	var first, rest httpx.List[jobs.LogLine]
	owner.Do(http.MethodGet, path+"/logs?limit=2", nil).RequireStatus(http.StatusOK).Decode(&first)
	if len(first.Items) != 2 || first.Items[1].Stream != "stderr" || first.NextCursor == nil || *first.NextCursor != "1.2" {
		t.Fatalf("first page %+v", first)
	}
	owner.Do(http.MethodGet, path+"/logs?after="+*first.NextCursor, nil).RequireStatus(http.StatusOK).Decode(&rest)
	if len(rest.Items) != 1 || rest.Items[0].Message != "giving up" {
		t.Fatalf("rest %+v", rest)
	}

	owner.Do(http.MethodGet, path+"/logs?after=nope", nil).RequireStatus(http.StatusBadRequest)
	outsider.Do(http.MethodGet, path, nil).RequireStatus(http.StatusNotFound)
	outsider.Do(http.MethodGet, path+"/logs", nil).RequireStatus(http.StatusNotFound)
}
