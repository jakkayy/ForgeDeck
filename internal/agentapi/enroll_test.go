package agentapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

func enroll(env *apitest.Env, token string, protocol int) *apitest.Response {
	return env.Anonymous().Do(http.MethodPost, "/api/v1/agent/enroll", agentproto.EnrollRequest{
		Token: token, AgentVersion: "0.1.0", ProtocolVersion: protocol,
	})
}

func TestEnrollRegistersServerOnce(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	token := owner.IssueEnrollmentToken(projectID, "prod-01")

	var resp agentproto.EnrollResponse
	enroll(env, token, agentproto.ProtocolVersion).RequireStatus(http.StatusCreated).Decode(&resp)
	if !strings.HasPrefix(resp.Credential, agentproto.CredentialPrefix) {
		t.Fatalf("credential %q", resp.Credential)
	}

	var name, version string
	err := env.Pool.QueryRow(context.Background(),
		"SELECT name, agent_version FROM servers WHERE id = $1 AND project_id = $2", resp.ServerID, projectID).Scan(&name, &version)
	if err != nil || name != "prod-01" || version != "0.1.0" {
		t.Fatalf("server row: %q %q %v", name, version, err)
	}

	enroll(env, token, agentproto.ProtocolVersion).RequireStatus(http.StatusUnauthorized)
}

func TestEnrollRejectsExpiredToken(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	token := owner.IssueEnrollmentToken(owner.CreateProject("Infra"), "prod-01")
	if _, err := env.Pool.Exec(context.Background(), "UPDATE enrollment_tokens SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	enroll(env, token, agentproto.ProtocolVersion).RequireStatus(http.StatusUnauthorized)
}

func TestEnrollRejectsUnsupportedProtocol(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	token := owner.IssueEnrollmentToken(owner.CreateProject("Infra"), "prod-01")

	resp := enroll(env, token, agentproto.ProtocolVersion+1).RequireStatus(http.StatusBadRequest)
	if resp.ErrorCode() != "unsupported_protocol" {
		t.Fatalf("code %s", resp.ErrorCode())
	}
	enroll(env, token, agentproto.ProtocolVersion).RequireStatus(http.StatusCreated)
}

func TestEnrollWithDuplicateNameKeepsTokenUsable(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	env.EnrollAgent(owner, projectID, "prod-01")
	second := owner.IssueEnrollmentToken(projectID, "prod-01")

	enroll(env, second, agentproto.ProtocolVersion).RequireStatus(http.StatusConflict)

	var used bool
	if err := env.Pool.QueryRow(context.Background(),
		"SELECT used_at IS NOT NULL FROM enrollment_tokens WHERE server_id IS NULL").Scan(&used); err != nil || used {
		t.Fatalf("token consumed by failed enrollment: used=%v err=%v", used, err)
	}
}

func TestProjectWithServersCannotBeDeleted(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	env.EnrollAgent(owner, projectID, "prod-01")

	var slug string
	if err := env.Pool.QueryRow(context.Background(), "SELECT slug FROM projects WHERE id = $1", projectID).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	resp := owner.Do(http.MethodDelete, "/api/v1/projects/"+projectID.String(), map[string]string{"confirm": slug}).
		RequireStatus(http.StatusConflict)
	if resp.ErrorCode() != "project_has_servers" {
		t.Fatalf("code %s", resp.ErrorCode())
	}
}
