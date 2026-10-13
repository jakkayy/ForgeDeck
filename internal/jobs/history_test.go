package jobs_test

import (
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

func TestJobHistoryPagesFiltersAndScopes(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	other, _ := env.NewUser("other@example.com")
	projectID := owner.CreateProject("Infra")
	otherProject := other.CreateProject("Theirs")
	agent := env.EnrollAgent(owner, projectID, "prod-01")
	theirAgent := env.EnrollAgent(other, otherProject, "their-01")
	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)

	for i := range 5 {
		jobType := agentproto.JobFetchLogs
		if i%2 == 0 {
			jobType = agentproto.JobContainerAction
		}
		if _, err := svc.Enqueue(t.Context(), jobs.NewJob{ProjectID: projectID, Type: jobType, WorkerKind: jobs.WorkerAgent,
			TargetServerID: &agent.ServerID, Payload: map[string]int{"n": i}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Enqueue(t.Context(), jobs.NewJob{ProjectID: otherProject, Type: agentproto.JobFetchLogs,
		WorkerKind: jobs.WorkerAgent, TargetServerID: &theirAgent.ServerID, Payload: map[string]int{}}); err != nil {
		t.Fatal(err)
	}

	var seen []jobs.Job
	cursor := ""
	for {
		var page httpx.List[jobs.Job]
		owner.Do(http.MethodGet, "/api/v1/jobs?limit=2&cursor="+cursor, nil).RequireStatus(http.StatusOK).Decode(&page)
		seen = append(seen, page.Items...)
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("owner sees %d jobs, want their 5 only", len(seen))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i].CreatedAt.After(seen[i-1].CreatedAt) {
			t.Fatal("history is not newest first")
		}
	}

	var actions httpx.List[jobs.Job]
	owner.Do(http.MethodGet, "/api/v1/jobs?type=container_action&project_id="+projectID.String(), nil).
		RequireStatus(http.StatusOK).Decode(&actions)
	if len(actions.Items) != 3 {
		t.Fatalf("%d container actions, want 3", len(actions.Items))
	}

	var foreign httpx.List[jobs.Job]
	owner.Do(http.MethodGet, "/api/v1/jobs?project_id="+otherProject.String(), nil).RequireStatus(http.StatusOK).Decode(&foreign)
	if len(foreign.Items) != 0 {
		t.Fatalf("owner sees %d jobs of a foreign project", len(foreign.Items))
	}

	owner.Do(http.MethodGet, "/api/v1/jobs?status=exploded", nil).RequireStatus(http.StatusBadRequest)
	owner.Do(http.MethodGet, "/api/v1/jobs?cursor=%21%21", nil).RequireStatus(http.StatusBadRequest)
}
