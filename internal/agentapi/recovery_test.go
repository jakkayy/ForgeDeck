package agentapi_test

import (
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

// TestLostAttemptIsRetriedAndItsLateResultRejected: a lost attempt's late result is refused.
func TestLostAttemptIsRetriedAndItsLateResultRejected(t *testing.T) {
	f := newClaimFixture(t)
	jobID := f.enqueue(t, f.agent.ServerID)

	// fetch_logs allows two attempts; container_action allows one.
	if _, err := f.env.Pool.Exec(t.Context(), "UPDATE jobs SET type = $1, max_attempts = 2 WHERE id = $2",
		agentproto.JobFetchLogs, jobID); err != nil {
		t.Fatal(err)
	}

	var first agentproto.Job
	claim(f.agent, 0).RequireStatus(http.StatusOK).Decode(&first)
	if _, err := f.env.Pool.Exec(t.Context(), "UPDATE jobs SET lease_expires_at = now() - interval '1 second' WHERE id = $1", jobID); err != nil {
		t.Fatal(err)
	}
	if requeued, _, err := f.jobs.RecoverExpiredLeases(t.Context()); err != nil || requeued != 1 {
		t.Fatalf("recovery: requeued=%d err=%v", requeued, err)
	}

	var second agentproto.Job
	claim(f.agent, 0).RequireStatus(http.StatusOK).Decode(&second)
	if second.ID != jobID || second.Attempt != 2 {
		t.Fatalf("retry claimed %+v", second)
	}
	complete(f.agent, second, agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded}).RequireStatus(http.StatusNoContent)

	// the late result must not overwrite the retry's.
	late := complete(f.agent, first, agentproto.CompleteRequest{Outcome: agentproto.OutcomeFailed, ErrorCode: "late"})
	if late.Status != http.StatusConflict || late.ErrorCode() != "lease_lost" {
		t.Fatalf("late result: %d %s", late.Status, late.Body)
	}

	rows, err := f.env.Pool.Query(t.Context(), "SELECT attempt_number, status FROM job_attempts WHERE job_id = $1 ORDER BY attempt_number", jobID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var history []string
	for rows.Next() {
		var n int
		var status string
		if err := rows.Scan(&n, &status); err != nil {
			t.Fatal(err)
		}
		history = append(history, status)
	}
	if len(history) != 2 || history[0] != "lost" || history[1] != "succeeded" {
		t.Fatalf("attempt history %v, want [lost succeeded]", history)
	}
	var status string
	if err := f.env.Pool.QueryRow(t.Context(), "SELECT status FROM jobs WHERE id = $1", jobID).Scan(&status); err != nil || status != "succeeded" {
		t.Fatalf("job status %q %v", status, err)
	}
}
