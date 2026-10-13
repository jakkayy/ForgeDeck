package jobs_test

import (
	"context"
	"testing"
)

func TestClaimForAgentIsTargetedAndOrdered(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other := f.addServer(t, "prod-02")

	first, _ := f.svc.Enqueue(ctx, f.agentJob(f.serverID))
	second, _ := f.svc.Enqueue(ctx, f.agentJob(f.serverID))

	if job, err := f.svc.ClaimForAgent(ctx, other, "agent-b"); err != nil || job != nil {
		t.Fatalf("other server claimed %+v (err %v)", job, err)
	}

	job, err := f.svc.ClaimForAgent(ctx, f.serverID, "agent-a")
	if err != nil || job == nil {
		t.Fatalf("claim: %+v %v", job, err)
	}
	if job.ID != first.ID || job.Attempt != 1 || job.Type != "container_action" {
		t.Fatalf("claimed %+v, want first job attempt 1", job)
	}

	var status, owner string
	var attempts int
	if err := f.pool.QueryRow(ctx, `
		SELECT status, lease_owner, (SELECT count(*) FROM job_attempts WHERE job_id = jobs.id)
		FROM jobs WHERE id = $1`, job.ID).Scan(&status, &owner, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "running" || owner != "agent-a" || attempts != 1 {
		t.Fatalf("stored status=%s owner=%s attempts=%d", status, owner, attempts)
	}

	next, _ := f.svc.ClaimForAgent(ctx, f.serverID, "agent-a")
	if next == nil || next.ID != second.ID {
		t.Fatalf("second claim %+v, want second job", next)
	}
	if none, _ := f.svc.ClaimForAgent(ctx, f.serverID, "agent-a"); none != nil {
		t.Fatalf("queue should be empty, got %+v", none)
	}
}
