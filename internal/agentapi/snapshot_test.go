package agentapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

const (
	apiID    = "aaaaaaaaaaaa1111"
	workerID = "bbbbbbbbbbbb2222"
)

func container(id, name, state string) agentproto.Container {
	return agentproto.Container{ID: id, Name: name, Image: "ghcr.io/example/" + name + ":1", State: state, StatusText: "Up 2 hours"}
}

func sendSnapshot(t *testing.T, agent apitest.Agent, observedAt time.Time, containers ...agentproto.Container) bool {
	t.Helper()
	var resp agentproto.SnapshotResponse
	agent.Do(http.MethodPost, "/api/v1/agent/containers/snapshot", agentproto.ContainerSnapshot{
		ObservedAt: observedAt, Containers: containers,
	}).RequireStatus(http.StatusOK).Decode(&resp)
	return resp.Accepted
}

func storedNames(t *testing.T, env *apitest.Env, agent apitest.Agent) []string {
	t.Helper()
	rows, err := env.Pool.Query(context.Background(),
		"SELECT name FROM server_containers WHERE server_id = $1 ORDER BY name", agent.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	return names
}

func newAgent(t *testing.T) (*apitest.Env, apitest.Agent) {
	t.Helper()
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	return env, env.EnrollAgent(owner, owner.CreateProject("Infra"), "prod-01")
}

func TestSnapshotReplacesContainers(t *testing.T) {
	env, agent := newAgent(t)
	t0 := time.Now().Add(-time.Minute)

	if !sendSnapshot(t, agent, t0, container(apiID, "api", "running"), container(workerID, "worker", "exited")) {
		t.Fatal("first snapshot not accepted")
	}
	if got := storedNames(t, env, agent); !slices.Equal(got, []string{"api", "worker"}) {
		t.Fatalf("stored %v", got)
	}

	if !sendSnapshot(t, agent, t0.Add(time.Second), container(apiID, "api", "running")) {
		t.Fatal("newer snapshot not accepted")
	}
	if got := storedNames(t, env, agent); !slices.Equal(got, []string{"api"}) {
		t.Fatalf("vanished container kept: %v", got)
	}

	if !sendSnapshot(t, agent, t0.Add(2*time.Second)) {
		t.Fatal("empty snapshot not accepted")
	}
	if got := storedNames(t, env, agent); len(got) != 0 {
		t.Fatalf("empty snapshot left %v", got)
	}
}

func TestSnapshotIgnoresOlderSnapshot(t *testing.T) {
	env, agent := newAgent(t)
	t0 := time.Now().Add(-time.Minute)

	sendSnapshot(t, agent, t0, container(apiID, "api", "running"))
	if sendSnapshot(t, agent, t0.Add(-time.Second), container(workerID, "worker", "running")) {
		t.Fatal("older snapshot accepted")
	}
	if sendSnapshot(t, agent, t0, container(workerID, "worker", "running")) {
		t.Fatal("same-time snapshot accepted")
	}
	if got := storedNames(t, env, agent); !slices.Equal(got, []string{"api"}) {
		t.Fatalf("stored %v", got)
	}
}

func TestSnapshotClampsFutureObservedAt(t *testing.T) {
	env, agent := newAgent(t)
	sendSnapshot(t, agent, time.Now().Add(24*time.Hour), container(apiID, "api", "running"))

	var observed time.Time
	if err := env.Pool.QueryRow(context.Background(),
		"SELECT containers_observed_at FROM servers WHERE id = $1", agent.ServerID).Scan(&observed); err != nil {
		t.Fatal(err)
	}
	if time.Until(observed) > time.Minute {
		t.Fatalf("future observed_at stored: %v", observed)
	}
}

func TestSnapshotValidation(t *testing.T) {
	_, agent := newAgent(t)
	cases := map[string]agentproto.ContainerSnapshot{
		"missing observed_at": {Containers: []agentproto.Container{container(apiID, "api", "running")}},
		"bad container id":    {ObservedAt: time.Now(), Containers: []agentproto.Container{container("../etc", "api", "running")}},
		"unknown state":       {ObservedAt: time.Now(), Containers: []agentproto.Container{container(apiID, "api", "zombie")}},
		"duplicate id": {ObservedAt: time.Now(), Containers: []agentproto.Container{
			container(apiID, "api", "running"), container(apiID, "api2", "running"),
		}},
	}
	for name, snapshot := range cases {
		t.Run(name, func(t *testing.T) {
			agent.Do(http.MethodPost, "/api/v1/agent/containers/snapshot", snapshot).RequireStatus(http.StatusUnprocessableEntity)
		})
	}
}
