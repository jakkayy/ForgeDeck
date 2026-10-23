package runners_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/overview"
	"github.com/jakkayy/ForgeDeck/internal/runners"
)

type created struct {
	runners.Runner
	Credential string `json:"credential"`
}

func TestOnlyAnAdministratorManagesRunners(t *testing.T) {
	env := apitest.New(t)
	admin, _ := env.NewUser("admin@example.com") // the first account is the administrator
	other, _ := env.NewUser("other@example.com")
	anonymous := env.Anonymous()

	for name, call := range map[string]func(c *apitest.Client) *apitest.Response{
		"list": func(c *apitest.Client) *apitest.Response { return c.Do(http.MethodGet, "/api/v1/runners", nil) },
		"create": func(c *apitest.Client) *apitest.Response {
			return c.Do(http.MethodPost, "/api/v1/runners", map[string]string{"name": "build-01"})
		},
		"delete": func(c *apitest.Client) *apitest.Response {
			return c.Do(http.MethodDelete, "/api/v1/runners/00000000-0000-4000-8000-000000000000", nil)
		},
	} {
		call(anonymous).RequireStatus(http.StatusUnauthorized)
		call(other).RequireStatus(http.StatusForbidden)
		t.Logf("%s: anonymous 401, non-administrator 403", name)
	}
	if n := countRunners(t, env); n != 0 {
		t.Fatalf("a refused request created %d runners", n)
	}
	admin.Do(http.MethodGet, "/api/v1/runners", nil).RequireStatus(http.StatusOK)
}

func countRunners(t *testing.T, env *apitest.Env) int {
	t.Helper()
	var n int
	if err := env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM runners`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateListAndRevokeARunner(t *testing.T) {
	env := apitest.New(t)
	admin, _ := env.NewUser("admin@example.com")

	var made created
	resp := admin.Do(http.MethodPost, "/api/v1/runners", map[string]string{"name": "build-01"}).RequireStatus(http.StatusCreated)
	resp.Decode(&made)
	if !strings.HasPrefix(made.Credential, runners.CredentialPrefix) || made.Name != "build-01" || made.Status != runners.StatusPending {
		t.Fatalf("created runner %+v", made)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("the response carrying the credential may be cached")
	}

	// The list never carries a credential.
	listed := admin.Do(http.MethodGet, "/api/v1/runners", nil).RequireStatus(http.StatusOK)
	if strings.Contains(string(listed.Body), "credential") || strings.Contains(string(listed.Body), made.Credential) {
		t.Fatalf("the list leaks a credential: %s", listed.Body)
	}

	admin.Do(http.MethodPost, "/api/v1/runners", map[string]string{"name": "build-01"}).RequireStatus(http.StatusConflict)
	bad := admin.Do(http.MethodPost, "/api/v1/runners", map[string]string{"name": "Build 01"}).RequireStatus(http.StatusUnprocessableEntity)
	if !strings.Contains(string(bad.Body), `"name"`) {
		t.Errorf("the invalid name is not reported on its field: %s", bad.Body)
	}

	// The credential works for the Runner, and the dashboard then shows it online.
	runner := env.AgentClient(made.Credential)
	runner.Do(http.MethodPost, "/api/v1/runner/heartbeat", agentproto.HeartbeatRequest{
		AgentVersion: "0.1.0", ProtocolVersion: agentproto.ProtocolVersion,
	}).RequireStatus(http.StatusOK)
	var list struct{ Items []runners.Runner }
	admin.Do(http.MethodGet, "/api/v1/runners", nil).RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 1 || list.Items[0].Status != runners.StatusOnline || list.Items[0].Version == nil || *list.Items[0].Version != "0.1.0" {
		t.Fatalf("runner after a heartbeat: %+v", list.Items)
	}

	var summary overview.Overview
	admin.Do(http.MethodGet, "/api/v1/overview", nil).RequireStatus(http.StatusOK).Decode(&summary)
	if summary.Runners != (overview.RunnerCounts{Total: 1, Online: 1}) {
		t.Fatalf("overview runners %+v", summary.Runners)
	}

	// Deleting revokes the credential at once.
	admin.Do(http.MethodDelete, "/api/v1/runners/"+made.ID.String(), nil).RequireStatus(http.StatusNoContent)
	runner.Do(http.MethodPost, "/api/v1/runner/heartbeat", agentproto.HeartbeatRequest{
		AgentVersion: "0.1.0", ProtocolVersion: agentproto.ProtocolVersion,
	}).RequireStatus(http.StatusUnauthorized)
	admin.Do(http.MethodDelete, "/api/v1/runners/"+made.ID.String(), nil).RequireStatus(http.StatusNotFound)
}

func TestRunnerEventsAreAuditedWithoutTheCredential(t *testing.T) {
	env := apitest.New(t)
	admin, _ := env.NewUser("admin@example.com")
	var made created
	admin.Do(http.MethodPost, "/api/v1/runners", map[string]string{"name": "build-01"}).RequireStatus(http.StatusCreated).Decode(&made)
	admin.Do(http.MethodDelete, "/api/v1/runners/"+made.ID.String(), nil).RequireStatus(http.StatusNoContent)

	rows, err := env.Pool.Query(context.Background(), `SELECT action, coalesce(target_name, ''), metadata::text FROM audit_logs WHERE action LIKE 'runner.%' ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var actions []string
	for rows.Next() {
		var action, name, metadata string
		if err := rows.Scan(&action, &name, &metadata); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(metadata, made.Credential) || strings.Contains(name, made.Credential) {
			t.Errorf("the audit event %s holds the credential", action)
		}
		actions = append(actions, action)
	}
	if strings.Join(actions, ",") != "runner.create,runner.delete" {
		t.Fatalf("audit events %v", actions)
	}
}
