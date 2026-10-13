package runner_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/runner"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

func noDocker(context.Context) (agentproto.Docker, *agentproto.Resources) {
	return agentproto.Docker{}, nil
}

func TestRunnerClaimsAndCompletesBuildJobs(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	registered := env.NewRunner("build-01")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	handlers := map[string]worker.Handler{
		agentproto.JobBuild: func(_ context.Context, _ json.RawMessage, log worker.Logger) (any, error) {
			log.Log("stdout", "built")
			return map[string]string{"image_digest": "sha256:abc"}, nil
		},
	}
	go func() {
		cfg := runner.Config{ControlPlaneURL: env.Server.URL, Credential: registered.Credential, Version: "test"}
		done <- runner.Run(ctx, cfg, handlers, noDocker, slog.New(slog.DiscardHandler))
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run: %v", err)
		}
	})

	svc := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now)
	job, err := svc.Enqueue(t.Context(), jobs.NewJob{ProjectID: projectID, Type: agentproto.JobBuild,
		WorkerKind: jobs.WorkerRunner, Payload: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var got jobs.Job
		owner.Do(http.MethodGet, "/api/v1/jobs/"+job.ID.String(), nil).RequireStatus(http.StatusOK).Decode(&got)
		if got.Status == jobs.StatusSucceeded {
			break
		}
		if got.Status == jobs.StatusFailed || time.Now().After(deadline) {
			t.Fatalf("build job %+v", got)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRunnerStopsWhenDeleted(t *testing.T) {
	env := apitest.New(t)
	cfg := runner.Config{ControlPlaneURL: env.Server.URL, Credential: "fdr_unknown", Version: "test"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runner.Run(ctx, cfg, runner.Handlers(), noDocker, slog.New(slog.DiscardHandler)); err == nil || ctx.Err() != nil {
		t.Fatalf("Run = %v, want an unauthorized error before the timeout", err)
	}
}
