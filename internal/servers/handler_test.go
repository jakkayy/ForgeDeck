package servers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

func TestCreateEnrollmentToken(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	path := "/api/v1/projects/" + projectID.String() + "/servers/enrollment-tokens"

	resp := owner.Do(http.MethodPost, path, map[string]string{"server_name": "prod-01"}).RequireStatus(http.StatusCreated)
	var token servers.EnrollmentToken
	resp.Decode(&token)

	if len(token.Token) < 40 || token.Token[:4] != "fde_" || token.ServerName != "prod-01" {
		t.Fatalf("got %+v", token)
	}
	if ttl := time.Until(token.ExpiresAt); ttl < 50*time.Minute || ttl > time.Hour {
		t.Fatalf("expires in %v, want about 1h", ttl)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("token response may be cached")
	}
}

func TestCreateEnrollmentTokenRules(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	dev, devID := env.NewUser("dev@example.com")
	outsider, _ := env.NewUser("outsider@example.com")
	projectID := owner.CreateProject("Infra")
	env.AddMember(projectID, devID, "developer")
	path := "/api/v1/projects/" + projectID.String() + "/servers/enrollment-tokens"

	dev.Do(http.MethodPost, path, map[string]string{"server_name": "x"}).RequireStatus(http.StatusForbidden)
	outsider.Do(http.MethodPost, path, map[string]string{"server_name": "x"}).RequireStatus(http.StatusNotFound)
	owner.Do(http.MethodPost, path, map[string]string{"server_name": " "}).RequireStatus(http.StatusUnprocessableEntity)
}

func TestServerListsAndDetail(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	outsider, _ := env.NewUser("outsider@example.com")
	infra := owner.CreateProject("Infra")
	web := owner.CreateProject("Web")
	prod := env.EnrollAgent(owner, infra, "prod-01")
	env.EnrollAgent(owner, web, "web-01")
	outsider.CreateProject("Elsewhere")

	prod.Do(http.MethodPost, "/api/v1/agent/heartbeat", agentproto.HeartbeatRequest{
		AgentVersion: "0.2.0", ProtocolVersion: agentproto.ProtocolVersion,
		Docker:    agentproto.Docker{Available: true, Version: "27.3.1"},
		Resources: &agentproto.Resources{CPUPercent: 10, MemoryPercent: 20, DiskPercent: 30},
	}).RequireStatus(http.StatusOK)

	var all httpx.List[servers.Server]
	owner.Do(http.MethodGet, "/api/v1/servers", nil).RequireStatus(http.StatusOK).Decode(&all)
	if len(all.Items) != 2 || all.Items[0].Name != "prod-01" || all.Items[0].ProjectName != "Infra" {
		t.Fatalf("all servers: %+v", all.Items)
	}

	var inProject httpx.List[servers.Server]
	owner.Do(http.MethodGet, "/api/v1/projects/"+web.String()+"/servers", nil).RequireStatus(http.StatusOK).Decode(&inProject)
	if len(inProject.Items) != 1 || inProject.Items[0].Name != "web-01" || inProject.Items[0].Status != servers.StatusPending {
		t.Fatalf("project servers: %+v", inProject.Items)
	}

	var detail servers.Server
	owner.Do(http.MethodGet, "/api/v1/servers/"+prod.ServerID.String(), nil).RequireStatus(http.StatusOK).Decode(&detail)
	if detail.Status != servers.StatusOnline || !detail.Docker.Available || detail.Resources == nil || detail.Resources.DiskPercent != 30 {
		t.Fatalf("detail: %+v", detail)
	}

	var none httpx.List[servers.Server]
	outsider.Do(http.MethodGet, "/api/v1/servers", nil).RequireStatus(http.StatusOK).Decode(&none)
	if len(none.Items) != 0 {
		t.Fatalf("outsider sees %+v", none.Items)
	}
	outsider.Do(http.MethodGet, "/api/v1/servers/"+prod.ServerID.String(), nil).RequireStatus(http.StatusNotFound)
	outsider.Do(http.MethodGet, "/api/v1/projects/"+infra.String()+"/servers", nil).RequireStatus(http.StatusNotFound)
}

func TestDeleteServerRevokesAgent(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	dev, devID := env.NewUser("dev@example.com")
	projectID := owner.CreateProject("Infra")
	env.AddMember(projectID, devID, "developer")
	agent := env.EnrollAgent(owner, projectID, "prod-01")
	path := "/api/v1/servers/" + agent.ServerID.String()
	hb := agentproto.HeartbeatRequest{AgentVersion: "0.1.0", ProtocolVersion: agentproto.ProtocolVersion}

	agent.Do(http.MethodPost, "/api/v1/agent/heartbeat", hb).RequireStatus(http.StatusOK)
	dev.Do(http.MethodDelete, path, nil).RequireStatus(http.StatusForbidden)

	owner.Do(http.MethodDelete, path, nil).RequireStatus(http.StatusNoContent)
	owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusNotFound)
	agent.Do(http.MethodPost, "/api/v1/agent/heartbeat", hb).RequireStatus(http.StatusUnauthorized)

	// The name is free again, so the same host can be re-enrolled.
	env.EnrollAgent(owner, projectID, "prod-01")
}
