package jobs_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

type fixture struct {
	pool      *pgxpool.Pool
	svc       *jobs.Service
	projectID uuid.UUID
	serverID  uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	f := fixture{pool: pool, svc: jobs.NewService(pool, authz.New(store.New(pool)), time.Now)}
	if err := pool.QueryRow(ctx, "INSERT INTO projects (name, slug) VALUES ('Infra', 'infra') RETURNING id").Scan(&f.projectID); err != nil {
		t.Fatal(err)
	}
	f.serverID = f.addServer(t, "prod-01")
	return f
}

func (f fixture) addServer(t *testing.T, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		"INSERT INTO servers (project_id, name) VALUES ($1, $2) RETURNING id", f.projectID, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f fixture) agentJob(serverID uuid.UUID) jobs.NewJob {
	return jobs.NewJob{
		ProjectID:      f.projectID,
		Type:           "container_action",
		WorkerKind:     jobs.WorkerAgent,
		TargetServerID: &serverID,
		Payload:        map[string]string{"action": "restart"},
	}
}

func TestEnqueueStoresQueuedJobAndNotifies(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	conn, err := f.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+jobs.NotifyChannel); err != nil {
		t.Fatal(err)
	}

	job, err := f.svc.Enqueue(ctx, f.agentJob(f.serverID))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if job.Status != string(jobs.StatusQueued) || job.MaxAttempts != 1 || payload["action"] != "restart" {
		t.Fatalf("job %+v payload %v", job, payload)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	n, err := conn.Conn().WaitForNotification(waitCtx)
	if err != nil {
		t.Fatalf("no notification: %v", err)
	}
	if n.Payload != f.serverID.String() {
		t.Fatalf("notification payload %q, want server ID", n.Payload)
	}
}

func TestEnqueueRejectsAgentJobWithoutTarget(t *testing.T) {
	f := newFixture(t)
	job := f.agentJob(f.serverID)
	job.TargetServerID = nil
	if _, err := f.svc.Enqueue(context.Background(), job); err == nil {
		t.Fatal("agent job without target was stored")
	}
}
