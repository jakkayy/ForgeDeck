package jobs_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

type sseEvent struct {
	name, id, data string
}

func readEvents(t *testing.T, resp *http.Response, events chan<- sseEvent) {
	t.Helper()
	defer close(events)
	defer func() { _ = resp.Body.Close() }()
	scanner := bufio.NewScanner(resp.Body)
	var current sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			current.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "id: "):
			current.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			current.data = strings.TrimPrefix(line, "data: ")
		case line == "" && current.name != "":
			events <- current
			if current.name == "end" {
				return
			}
			current = sseEvent{}
		}
	}
}

func next(t *testing.T, events <-chan sseEvent) sseEvent {
	t.Helper()
	select {
	case e, ok := <-events:
		if !ok {
			t.Fatal("stream closed early")
		}
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("no event within 5s")
	}
	return sseEvent{}
}

func statusOf(t *testing.T, e sseEvent) jobs.Status {
	t.Helper()
	var job jobs.Job
	if e.name != "status" || json.Unmarshal([]byte(e.data), &job) != nil {
		t.Fatalf("want status event, got %+v", e)
	}
	return job.Status
}

func TestJobEventStream(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	outsider, _ := env.NewUser("outsider@example.com")
	projectID := owner.CreateProject("Infra")
	agent := env.EnrollAgent(owner, projectID, "prod-01")
	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)
	job, err := svc.Enqueue(t.Context(), jobs.NewJob{ProjectID: projectID, Type: agentproto.JobFetchLogs,
		WorkerKind: jobs.WorkerAgent, TargetServerID: &agent.ServerID, Payload: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/jobs/" + job.ID.String() + "/events"

	resp := owner.Stream(path, nil) //nolint:bodyclose // closed by readEvents
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	events := make(chan sseEvent, 32)
	go readEvents(t, resp, events)
	if s := statusOf(t, next(t, events)); s != jobs.StatusQueued {
		t.Fatalf("first status %s", s)
	}

	wait := 0
	var claimed agentproto.Job
	agent.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusOK).Decode(&claimed)
	if s := statusOf(t, next(t, events)); s != jobs.StatusRunning {
		t.Fatalf("after claim: %s", s)
	}

	agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+job.ID.String()+"/events", agentproto.EventsRequest{
		Attempt: claimed.Attempt, Events: []agentproto.LogEvent{
			{Sequence: 1, Stream: "stdout", Message: "first"}, {Sequence: 2, Stream: "stdout", Message: "second"},
		}}).RequireStatus(http.StatusNoContent)
	if e := next(t, events); e.name != "log" || e.id != "1.1" || !strings.Contains(e.data, "first") {
		t.Fatalf("log event %+v", e)
	}
	if e := next(t, events); e.id != "1.2" {
		t.Fatalf("log event %+v", e)
	}

	agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+job.ID.String()+"/complete",
		agentproto.CompleteRequest{Attempt: claimed.Attempt, Outcome: agentproto.OutcomeSucceeded}).RequireStatus(http.StatusNoContent)
	if s := statusOf(t, next(t, events)); s != jobs.StatusSucceeded {
		t.Fatalf("after complete: %s", s)
	}
	if e := next(t, events); e.name != "end" {
		t.Fatalf("want end, got %+v", e)
	}

	resumed := make(chan sseEvent, 8)
	go readEvents(t, owner.Stream(path, map[string]string{"Last-Event-ID": "1.1"}), resumed) //nolint:bodyclose // closed by readEvents
	if e := next(t, resumed); e.id != "1.2" {
		t.Fatalf("resume started at %+v", e)
	}

	denied := outsider.Stream(path, nil)
	_ = denied.Body.Close()
	if denied.StatusCode != http.StatusNotFound {
		t.Fatalf("outsider got %d", denied.StatusCode)
	}
}
