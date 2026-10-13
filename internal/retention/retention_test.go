package retention_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/retention"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }

type world struct {
	pool      *pgxpool.Pool
	projectID uuid.UUID
	userID    uuid.UUID
}

func newWorld(t *testing.T) world {
	t.Helper()
	pool := testdb.New(t)
	w := world{pool: pool}
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (email, password_hash) VALUES ('u@example.com', 'x') RETURNING id`).Scan(&w.userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `INSERT INTO projects (name, slug) VALUES ('P', 'p') RETURNING id`).Scan(&w.projectID); err != nil {
		t.Fatal(err)
	}
	return w
}

// job inserts a build job in status that finished daysAgo, with three log lines.
func (w world) job(t *testing.T, status string, finishedDaysAgo int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	var finished *time.Time
	if finishedDaysAgo >= 0 {
		f := daysAgo(finishedDaysAgo)
		finished = &f
	}
	err := w.pool.QueryRow(context.Background(), `INSERT INTO jobs (project_id, type, worker_kind, status, payload, finished_at, created_at)
		VALUES ($1, 'build', 'runner', $2, '{}', $3, $4) RETURNING id`, w.projectID, status, finished, daysAgo(max(finishedDaysAgo, 0)+1)).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	for seq := 1; seq <= 3; seq++ {
		if _, err := w.pool.Exec(context.Background(), `INSERT INTO job_log_chunks (job_id, attempt_number, sequence_number, stream, content)
			VALUES ($1, 1, $2, 'stdout', 'line')`, id, seq); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (w world) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := w.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func run(t *testing.T, w world, policy retention.Policy) retention.Result {
	t.Helper()
	result, err := retention.NewService(store.New(w.pool), policy, func() time.Time { return now }).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOldJobLogsAreRemovedButTheJobsStay(t *testing.T) {
	w := newWorld(t)
	old := w.job(t, "succeeded", 40)
	recent := w.job(t, "succeeded", 5)
	running := w.job(t, "running", -1)
	failedOld := w.job(t, "failed", 31)

	result := run(t, w, retention.Policy{JobLogDays: 30})
	if result.JobsWithExpiredLogs != 2 || result.JobLogLines != 6 {
		t.Fatalf("result %+v", result)
	}
	lines := func(id uuid.UUID) int {
		return w.count(t, `SELECT count(*) FROM job_log_chunks WHERE job_id = $1`, id)
	}
	if lines(old) != 0 || lines(failedOld) != 0 || lines(recent) != 3 || lines(running) != 3 {
		t.Fatalf("lines: old %d, failedOld %d, recent %d, running %d", lines(old), lines(failedOld), lines(recent), lines(running))
	}
	if n := w.count(t, `SELECT count(*) FROM jobs`); n != 4 {
		t.Fatalf("%d jobs left, want all 4", n)
	}
	if n := w.count(t, `SELECT count(*) FROM jobs WHERE logs_expired_at IS NOT NULL`); n != 2 {
		t.Fatalf("%d jobs marked, want 2", n)
	}

	// A second pass does nothing more.
	if again := run(t, w, retention.Policy{JobLogDays: 30}); again != (retention.Result{}) {
		t.Fatalf("second pass removed %+v", again)
	}
}

func TestOldFinishedJobsAreDeletedButNeverOnesInProgress(t *testing.T) {
	w := newWorld(t)
	old := w.job(t, "succeeded", 200)
	stuck := w.job(t, "running", -1) // never finished: not ours to delete
	queued := w.job(t, "queued", -1)
	recent := w.job(t, "failed", 10)

	result := run(t, w, retention.Policy{JobDays: 180})
	if result.Jobs != 1 {
		t.Fatalf("result %+v", result)
	}
	for id, want := range map[uuid.UUID]int{old: 0, stuck: 1, queued: 1, recent: 1} {
		if got := w.count(t, `SELECT count(*) FROM jobs WHERE id = $1`, id); got != want {
			t.Errorf("job %s: %d rows, want %d", id, got, want)
		}
	}
	// A deleted job takes its log lines with it.
	if n := w.count(t, `SELECT count(*) FROM job_log_chunks WHERE job_id = $1`, old); n != 0 {
		t.Fatalf("%d log lines of the deleted job remain", n)
	}
}

func TestReleasesAndDeploymentsOutliveTheirJobs(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	var appID, connID, serverID, envID, deploymentID uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(w.pool.QueryRow(ctx, `INSERT INTO git_connections (project_id, provider, display_name, base_url, token_ciphertext, token_key_id, webhook_ciphertext, webhook_key_id)
		VALUES ($1, 'gitlab', 'g', 'https://gitlab.example.com', '\x00', 'v1', '\x00', 'v1') RETURNING id`, w.projectID).Scan(&connID))
	must(w.pool.QueryRow(ctx, `INSERT INTO applications (project_id, name, git_connection_id, repository_id, repository_full_name, branch)
		VALUES ($1, 'API', $2, '1', 'a/b', 'main') RETURNING id`, w.projectID, connID).Scan(&appID))
	must(w.pool.QueryRow(ctx, `INSERT INTO servers (project_id, name) VALUES ($1, 's') RETURNING id`, w.projectID).Scan(&serverID))
	config := `{"restart":"no"}`
	must(w.pool.QueryRow(ctx, `INSERT INTO environments (application_id, name, server_id, config) VALUES ($1, 'prod', $2, $3::jsonb) RETURNING id`,
		appID, serverID, config).Scan(&envID))
	buildJob := w.job(t, "succeeded", 400)
	ref := "r/i@sha256:" + strings.Repeat("a", 64)
	must(w.pool.QueryRow(ctx, `INSERT INTO deployments (environment_id, source_build_job_id, commit_sha, image_ref, config, status)
		VALUES ($1, $2, 'abc', $3, $4::jsonb, 'succeeded') RETURNING id`, envID, buildJob, ref, config).Scan(&deploymentID))
	_, err := w.pool.Exec(ctx, `INSERT INTO releases (application_id, environment_id, deployment_id, commit_sha, image_ref, config)
		VALUES ($1, $2, $3, 'abc', $4, $5::jsonb)`, appID, envID, deploymentID, ref, config)
	must(err)

	run(t, w, retention.Policy{JobLogDays: 30, JobDays: 180, AuditDays: 1})

	if n := w.count(t, `SELECT count(*) FROM jobs WHERE id = $1`, buildJob); n != 0 {
		t.Fatal("the old build job was not deleted")
	}
	if n := w.count(t, `SELECT count(*) FROM releases`); n != 1 {
		t.Fatalf("%d releases, want the one rollback needs", n)
	}
	if n := w.count(t, `SELECT count(*) FROM deployments WHERE id = $1 AND source_build_job_id IS NULL`, deploymentID); n != 1 {
		t.Fatal("the deployment is gone or still points at a deleted job")
	}
}

func TestExpiredSessionsTokensWebhooksAndAuditEventsAreRemoved(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := w.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	session := func(hash string, expiresDaysAgo int) {
		exec(`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, w.userID, []byte(hash), daysAgo(expiresDaysAgo))
	}
	session("expired-long-ago", 10)
	session("expired-just-now", 0) // within the day of grace
	exec(`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, 'valid', $2)`, w.userID, now.Add(24*time.Hour))

	token := func(hash string, usedDaysAgo, expiresDaysAgo int) {
		var used *time.Time
		if usedDaysAgo >= 0 {
			u := daysAgo(usedDaysAgo)
			used = &u
		}
		exec(`INSERT INTO enrollment_tokens (project_id, server_name, token_hash, expires_at, used_at) VALUES ($1, 'x', $2, $3, $4)`,
			w.projectID, []byte(hash), daysAgo(expiresDaysAgo), used)
	}
	token("used-long-ago", 30, 29)
	token("expired-long-ago", -1, 30)
	token("used-yesterday", 1, 0)
	exec(`INSERT INTO enrollment_tokens (project_id, server_name, token_hash, expires_at) VALUES ($1, 'x', 'pending', $2)`, w.projectID, now.Add(time.Hour))

	exec(`INSERT INTO webhook_events (provider, delivery_id, event_type, received_at) VALUES ('gitlab', 'old', 'push', $1), ('gitlab', 'new', 'push', $2)`, daysAgo(40), daysAgo(2))
	exec(`INSERT INTO audit_logs (project_id, actor, action, target_type, created_at) VALUES ($1, 'a', 'x.old', 't', $2), ($1, 'a', 'x.new', 't', $3)`,
		w.projectID, daysAgo(400), daysAgo(2))

	result := run(t, w, retention.DefaultPolicy)
	if result.Sessions != 1 || result.EnrollmentTokens != 2 || result.WebhookEvents != 1 || result.AuditEvents != 1 {
		t.Fatalf("result %+v", result)
	}
	if n := w.count(t, `SELECT count(*) FROM sessions`); n != 2 {
		t.Errorf("%d sessions left, want the valid one and the one within grace", n)
	}
	if n := w.count(t, `SELECT count(*) FROM enrollment_tokens`); n != 2 {
		t.Errorf("%d tokens left, want the pending and the used-yesterday one", n)
	}
	if n := w.count(t, `SELECT count(*) FROM webhook_events WHERE delivery_id = 'new'`); n != 1 {
		t.Error("a recent webhook delivery was removed")
	}
	if n := w.count(t, `SELECT count(*) FROM audit_logs WHERE action = 'x.new'`); n != 1 {
		t.Error("a recent audit event was removed")
	}
}

func TestZeroKeepsEverythingAndTheCleanupWorksInBatches(t *testing.T) {
	w := newWorld(t)
	w.job(t, "succeeded", 1000)
	if got := run(t, w, retention.Policy{}); got.Jobs != 0 || got.JobsWithExpiredLogs != 0 {
		t.Fatalf("a zero policy removed %+v", got)
	}
	if n := w.count(t, `SELECT count(*) FROM job_log_chunks`); n != 3 {
		t.Fatalf("%d log lines left, want 3", n)
	}

	// More old jobs than one batch.
	for range 2100 {
		if _, err := w.pool.Exec(context.Background(), `INSERT INTO jobs (project_id, type, worker_kind, status, payload, finished_at)
			VALUES ($1, 'build', 'runner', 'succeeded', '{}', $2)`, w.projectID, daysAgo(500)); err != nil {
			t.Fatal(err)
		}
	}
	if got := run(t, w, retention.Policy{JobDays: 180}); got.Jobs != 2101 {
		t.Fatalf("removed %d jobs, want all 2101 over two batches", got.Jobs)
	}
}

func TestPolicyValidation(t *testing.T) {
	bad := map[string]retention.Policy{
		"negative":                   {JobLogDays: -1},
		"logs outlive their jobs":    {JobLogDays: 90, JobDays: 30},
		"negative audit retention":   {AuditDays: -5},
		"negative webhook retention": {WebhookDays: -1},
	}
	for name, policy := range bad {
		if policy.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, ok := range []retention.Policy{{}, retention.DefaultPolicy, {JobLogDays: 30}, {JobLogDays: 30, JobDays: 30}} {
		if err := ok.Validate(); err != nil {
			t.Errorf("%+v: %v", ok, err)
		}
	}
}
