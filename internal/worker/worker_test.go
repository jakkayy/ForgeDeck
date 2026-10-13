package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

type workerFixture struct {
	env       *apitest.Env
	owner     *apitest.Client
	enrolled  apitest.Agent
	client    *worker.Client
	projectID uuid.UUID
}

func newWorkerFixture(t *testing.T) workerFixture {
	t.Helper()
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	enrolled := env.EnrollAgent(owner, projectID, "prod-01")
	return workerFixture{env: env, owner: owner, enrolled: enrolled, projectID: projectID,
		client: worker.NewClient(env.Server.URL, worker.AgentAPI, enrolled.Credential)}
}

func (f workerFixture) enqueue(t *testing.T, jobType string) uuid.UUID {
	t.Helper()
	svc := jobs.NewService(f.env.Pool, authz.New(store.New(f.env.Pool)), time.Now)
	job, err := svc.Enqueue(context.Background(), jobs.NewJob{
		ProjectID: f.projectID, Type: jobType, WorkerKind: jobs.WorkerAgent,
		TargetServerID: &f.enrolled.ServerID, Payload: map[string]string{"greeting": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return job.ID
}

func (f workerFixture) waitForJob(t *testing.T, id uuid.UUID) jobs.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var job jobs.Job
		f.owner.Do(http.MethodGet, "/api/v1/jobs/"+id.String(), nil).RequireStatus(http.StatusOK).Decode(&job)
		if job.Status == jobs.StatusSucceeded || job.Status == jobs.StatusFailed {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job never finished: %+v", job)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func startWorker(t *testing.T, f workerFixture, handlers map[string]worker.Handler, afterJob func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = worker.NewWorker(f.client, handlers, afterJob, slog.New(slog.DiscardHandler)).Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func TestWorkerRunsHandlersAndReportsOutcome(t *testing.T) {
	f := newWorkerFixture(t)
	afterJobs := make(chan struct{}, 10)
	startWorker(t, f, map[string]worker.Handler{
		"echo": func(_ context.Context, job *agentproto.Job, log worker.Logger) (any, error) {
			var p map[string]string
			_ = json.Unmarshal(job.Payload, &p)
			log.Log("stdout", p["greeting"])
			log.Log("system", "done")
			return map[string]string{"echoed": p["greeting"]}, nil
		},
		"boom": func(context.Context, *agentproto.Job, worker.Logger) (any, error) {
			return nil, &worker.JobError{Code: "container_not_found", Message: "No such container."}
		},
		"crash": func(context.Context, *agentproto.Job, worker.Logger) (any, error) {
			return nil, errors.New("docker daemon unreachable")
		},
	}, func() { afterJobs <- struct{}{} })

	echo := f.waitForJob(t, f.enqueue(t, "echo"))
	var result map[string]string
	_ = json.Unmarshal(echo.Result, &result)
	if echo.Status != jobs.StatusSucceeded || result["echoed"] != "hello" {
		t.Fatalf("echo job %+v", echo)
	}
	var logs httpx.List[jobs.LogLine]
	f.owner.Do(http.MethodGet, "/api/v1/jobs/"+echo.ID.String()+"/logs", nil).RequireStatus(http.StatusOK).Decode(&logs)
	if len(logs.Items) != 2 || logs.Items[0].Message != "hello" || logs.Items[1].Stream != "system" {
		t.Fatalf("logs %+v", logs.Items)
	}

	if boom := f.waitForJob(t, f.enqueue(t, "boom")); boom.Error == nil || boom.Error.Code != "container_not_found" {
		t.Fatalf("boom job %+v", boom)
	}
	if crash := f.waitForJob(t, f.enqueue(t, "crash")); crash.Error == nil || crash.Error.Code != "worker_error" {
		t.Fatalf("crash job %+v", crash)
	}
	if unknown := f.waitForJob(t, f.enqueue(t, "rotate_certificates")); unknown.Error == nil || unknown.Error.Code != "unsupported_job_type" {
		t.Fatalf("unknown job %+v", unknown)
	}
	if len(afterJobs) != 4 {
		t.Fatalf("afterJob ran %d times, want 4", len(afterJobs))
	}
}
