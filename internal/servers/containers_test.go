package servers_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

func TestListContainers(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	outsider, _ := env.NewUser("outsider@example.com")
	agent := env.EnrollAgent(owner, owner.CreateProject("Infra"), "prod-01")
	path := "/api/v1/servers/" + agent.ServerID.String() + "/containers"

	var empty servers.ContainerList
	owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK).Decode(&empty)
	if !empty.Stale || empty.ObservedAt != nil || len(empty.Containers) != 0 {
		t.Fatalf("before any snapshot: %+v", empty)
	}

	agent.Do(http.MethodPost, "/api/v1/agent/heartbeat", agentproto.HeartbeatRequest{
		AgentVersion: "0.1.0", ProtocolVersion: agentproto.ProtocolVersion,
	}).RequireStatus(http.StatusOK)
	agent.Do(http.MethodPost, "/api/v1/agent/containers/snapshot", agentproto.ContainerSnapshot{
		ObservedAt: time.Now(),
		Containers: []agentproto.Container{{
			ID: "aaaaaaaaaaaa1111", Name: "api", Image: "ghcr.io/example/api:1", State: "running",
			Labels: map[string]string{"forgedeck.application_id": "x"},
		}},
	}).RequireStatus(http.StatusOK)

	var fresh servers.ContainerList
	owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK).Decode(&fresh)
	if fresh.Stale || fresh.ObservedAt == nil || len(fresh.Containers) != 1 ||
		fresh.Containers[0].Name != "api" || fresh.Containers[0].Labels["forgedeck.application_id"] != "x" {
		t.Fatalf("fresh: %+v", fresh)
	}

	if _, err := env.Pool.Exec(context.Background(),
		"UPDATE servers SET containers_observed_at = now() - interval '10 minutes' WHERE id = $1", agent.ServerID); err != nil {
		t.Fatal(err)
	}
	var old servers.ContainerList
	owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK).Decode(&old)
	if !old.Stale {
		t.Fatal("old snapshot not marked stale")
	}

	outsider.Do(http.MethodGet, path, nil).RequireStatus(http.StatusNotFound)
}
