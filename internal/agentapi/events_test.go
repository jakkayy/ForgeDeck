package agentapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

func sendEvents(agent apitest.Agent, job agentproto.Job, events ...agentproto.LogEvent) *apitest.Response {
	return agent.Do(http.MethodPost, "/api/v1/agent/jobs/"+job.ID.String()+"/events",
		agentproto.EventsRequest{Attempt: job.Attempt, Events: events})
}

func line(seq int64, msg string) agentproto.LogEvent {
	return agentproto.LogEvent{Sequence: seq, Stream: agentproto.StreamStdout, Message: msg}
}

func TestAppendEventsIsIdempotent(t *testing.T) {
	f := newClaimFixture(t)
	f.enqueue(t, f.agent.ServerID)
	var job agentproto.Job
	claim(f.agent, 0).RequireStatus(http.StatusOK).Decode(&job)

	sendEvents(f.agent, job, line(1, "pulling image"), line(2, "starting")).RequireStatus(http.StatusNoContent)
	sendEvents(f.agent, job, line(2, "starting"), line(3, "healthy")).RequireStatus(http.StatusNoContent)

	rows, err := f.env.Pool.Query(context.Background(),
		"SELECT content FROM job_log_chunks WHERE job_id = $1 ORDER BY sequence_number", job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		lines = append(lines, s)
	}
	rows.Close()
	if strings.Join(lines, "|") != "pulling image|starting|healthy" {
		t.Fatalf("stored %v", lines)
	}
}

func TestAppendEventsRules(t *testing.T) {
	f := newClaimFixture(t)
	other := f.env.EnrollAgent(f.owner, f.projectID, "prod-02")
	f.enqueue(t, f.agent.ServerID)
	var job agentproto.Job
	claim(f.agent, 0).RequireStatus(http.StatusOK).Decode(&job)

	sendEvents(other, job, line(1, "x")).RequireStatus(http.StatusConflict)
	sendEvents(f.agent, job, agentproto.LogEvent{Sequence: 1, Stream: "tty", Message: "x"}).
		RequireStatus(http.StatusUnprocessableEntity)
	sendEvents(f.agent, job, line(1, strings.Repeat("a", agentproto.MaxEventBytes+1))).
		RequireStatus(http.StatusUnprocessableEntity)
}
