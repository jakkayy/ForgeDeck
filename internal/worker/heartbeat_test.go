package worker_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/servers"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

func enrolledClient(t *testing.T) (*apitest.Env, *apitest.Client, apitest.Agent, *worker.Client) {
	t.Helper()
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	enrolled := env.EnrollAgent(owner, owner.CreateProject("Infra"), "prod-01")
	return env, owner, enrolled, worker.NewClient(env.Server.URL, worker.AgentAPI, enrolled.Credential)
}

func dockerUp(context.Context) (agentproto.Docker, *agentproto.Resources) {
	return agentproto.Docker{Available: true, Version: "27.3.1"}, &agentproto.Resources{CPUPercent: 5, MemoryPercent: 40, DiskPercent: 60}
}

func TestHeartbeaterMarksServerOnline(t *testing.T) {
	_, owner, enrolled, client := enrolledClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- worker.NewHeartbeater(client, "0.3.0", dockerUp, slog.New(slog.DiscardHandler)).Run(ctx)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var s servers.Server
		owner.Do(http.MethodGet, "/api/v1/servers/"+enrolled.ServerID.String(), nil).RequireStatus(http.StatusOK).Decode(&s)
		if s.Status == servers.StatusOnline {
			if s.AgentVersion == nil || *s.AgentVersion != "0.3.0" || s.Resources == nil || s.Resources.DiskPercent != 60 {
				t.Fatalf("server %+v", s)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never came online: %+v", s)
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run after cancel: %v", err)
	}
}

func TestHeartbeaterStopsWhenServerRemoved(t *testing.T) {
	_, owner, enrolled, client := enrolledClient(t)
	owner.Do(http.MethodDelete, "/api/v1/servers/"+enrolled.ServerID.String(), nil).RequireStatus(http.StatusNoContent)

	err := worker.NewHeartbeater(client, "0.3.0", dockerUp, slog.New(slog.DiscardHandler)).Run(context.Background())
	if !errors.Is(err, worker.ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
}
