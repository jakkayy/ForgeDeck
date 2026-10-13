package jobs_test

import (
	"context"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/jobs"
)

func TestRecoverExpiredLeases(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	retryable := f.agentJob(f.serverID)
	retryable.MaxAttempts = 2
	first, _ := f.svc.Enqueue(ctx, retryable)
	last, _ := f.svc.Enqueue(ctx, f.agentJob(f.serverID)) // one attempt only
	fresh := f.agentJob(f.serverID)
	fresh.MaxAttempts = 2

	for range 2 {
		if job, err := f.svc.ClaimForAgent(ctx, f.serverID, "agent-a"); err != nil || job == nil {
			t.Fatalf("claim: %v %v", job, err)
		}
	}
	healthy, _ := f.svc.Enqueue(ctx, fresh)
	if _, err := f.svc.ClaimForAgent(ctx, f.serverID, "agent-a"); err != nil {
		t.Fatal(err)
	}
	// The first two attempts' workers vanished; the third is still renewing.
	if _, err := f.pool.Exec(ctx, "UPDATE jobs SET lease_expires_at = now() - interval '1 second' WHERE id = ANY($1)",
		[]any{first.ID, last.ID}); err != nil {
		t.Fatal(err)
	}

	requeued, failed, err := f.svc.RecoverExpiredLeases(ctx)
	if err != nil || requeued != 1 || failed != 1 {
		t.Fatalf("requeued=%d failed=%d err=%v", requeued, failed, err)
	}

	assertJob(t, f, first.ID.String(), "queued", "", "lost")
	assertJob(t, f, last.ID.String(), "failed", "lease_expired", "lost")
	assertJob(t, f, healthy.ID.String(), "running", "", "running")

	// The requeued job is claimable again as attempt 2.
	again, err := f.svc.ClaimForAgent(ctx, f.serverID, "agent-b")
	if err != nil || again == nil || again.ID != first.ID || again.Attempt != 2 {
		t.Fatalf("reclaim: %+v %v", again, err)
	}
	// The vanished worker's late renewal is refused.
	_, err = f.svc.RenewLease(ctx, jobs.Lease{JobID: first.ID, Attempt: 1, WorkerKind: jobs.WorkerAgent, ServerID: &f.serverID, WorkerID: "agent-a"})
	if err == nil {
		t.Fatal("stale attempt renewed its lease")
	}
}

func assertJob(t *testing.T, f fixture, id, wantStatus, wantError, wantFirstAttempt string) {
	t.Helper()
	var status, attempt string
	var errorCode *string
	err := f.pool.QueryRow(context.Background(), `
		SELECT jobs.status, jobs.error_code, job_attempts.status
		FROM jobs JOIN job_attempts ON job_attempts.job_id = jobs.id AND job_attempts.attempt_number = 1
		WHERE jobs.id = $1`, id).Scan(&status, &errorCode, &attempt)
	if err != nil {
		t.Fatal(err)
	}
	gotError := ""
	if errorCode != nil {
		gotError = *errorCode
	}
	if status != wantStatus || gotError != wantError || attempt != wantFirstAttempt {
		t.Fatalf("job %s: status=%s error=%s attempt=%s, want %s/%s/%s", id, status, gotError, attempt, wantStatus, wantError, wantFirstAttempt)
	}
}
