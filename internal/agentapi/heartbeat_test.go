package agentapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

func heartbeat(docker bool, resources *agentproto.Resources) agentproto.HeartbeatRequest {
	return agentproto.HeartbeatRequest{
		AgentVersion:    "0.2.0",
		ProtocolVersion: agentproto.ProtocolVersion,
		Docker:          agentproto.Docker{Available: docker, Version: "27.3.1"},
		Resources:       resources,
	}
}

func TestHeartbeatRecordsStatus(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	agent := env.EnrollAgent(owner, owner.CreateProject("Infra"), "prod-01")

	var resp agentproto.HeartbeatResponse
	agent.Do(http.MethodPost, "/api/v1/agent/heartbeat",
		heartbeat(true, &agentproto.Resources{CPUPercent: 21, MemoryPercent: 48, DiskPercent: 42})).
		RequireStatus(http.StatusOK).Decode(&resp)
	if resp.IntervalSeconds != 20 {
		t.Fatalf("interval %d", resp.IntervalSeconds)
	}

	var (
		lastSeen      time.Time
		version       string
		docker        bool
		cpu, mem, dsk float32
	)
	err := env.Pool.QueryRow(context.Background(), `
		SELECT last_seen_at, agent_version, docker_available, cpu_percent, memory_percent, disk_percent
		FROM servers WHERE id = $1`, agent.ServerID).Scan(&lastSeen, &version, &docker, &cpu, &mem, &dsk)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(lastSeen) > time.Minute || version != "0.2.0" || !docker || cpu != 21 || mem != 48 || dsk != 42 {
		t.Fatalf("stored %v %q %v %v %v %v", lastSeen, version, docker, cpu, mem, dsk)
	}
}

func TestHeartbeatValidation(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	agent := env.EnrollAgent(owner, owner.CreateProject("Infra"), "prod-01")

	agent.Do(http.MethodPost, "/api/v1/agent/heartbeat", heartbeat(true, &agentproto.Resources{CPUPercent: 150})).
		RequireStatus(http.StatusUnprocessableEntity)

	stale := heartbeat(true, nil)
	stale.ProtocolVersion = 0
	agent.Do(http.MethodPost, "/api/v1/agent/heartbeat", stale).RequireStatus(http.StatusBadRequest)

	env.Anonymous().Do(http.MethodPost, "/api/v1/agent/heartbeat", heartbeat(true, nil)).RequireStatus(http.StatusUnauthorized)
}
