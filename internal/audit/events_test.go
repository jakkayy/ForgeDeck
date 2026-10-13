package audit_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/audit"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// eventsOf returns the project's audit entries, newest first.
func eventsOf(t *testing.T, client *apitest.Client, projectID uuid.UUID) []audit.Entry {
	t.Helper()
	var page httpx.List[audit.Entry]
	list(client, projectID, "?limit=100").RequireStatus(http.StatusOK).Decode(&page)
	return page.Items
}

func find(entries []audit.Entry, action string) []audit.Entry {
	var out []audit.Entry
	for _, e := range entries {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

func meta(t *testing.T, e audit.Entry) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.Metadata, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestKeyActionsAreRecordedWithoutTheirSecrets(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	base := "/api/v1/projects/" + projectID.String()

	const secretValue, registryPassword = "audit-secret-value-123", "registry-password-456"
	owner.Do(http.MethodPut, base+"/secrets/DATABASE_PASSWORD", map[string]string{"value": secretValue}).RequireStatus(http.StatusCreated)
	owner.Do(http.MethodPut, base+"/secrets/DATABASE_PASSWORD", map[string]string{"value": "another-secret-value"}).RequireStatus(http.StatusOK)
	owner.Do(http.MethodPost, base+"/registries", map[string]string{"name": "GHCR", "host": "ghcr.io", "username": "bot", "password": registryPassword}).
		RequireStatus(http.StatusCreated)
	token := owner.IssueEnrollmentToken(projectID, "prod-01")
	owner.Do(http.MethodPatch, base, map[string]string{"name": "Shop Renamed"}).RequireStatus(http.StatusOK)
	other, _ := env.NewUser("teammate@example.com")
	owner.Do(http.MethodPost, base+"/members", map[string]string{"email": "teammate@example.com", "role": "developer"}).RequireStatus(http.StatusCreated)
	_ = other
	owner.Do(http.MethodDelete, base+"/secrets/DATABASE_PASSWORD", nil).RequireStatus(http.StatusNoContent)

	entries := eventsOf(t, owner, projectID)
	sets := find(entries, "secret.set")
	if len(sets) != 2 || sets[0].Actor != "owner@example.com" || sets[0].TargetName == nil || *sets[0].TargetName != "DATABASE_PASSWORD" {
		t.Fatalf("secret.set events %+v", sets)
	}
	if created, _ := meta(t, sets[1])["created"].(bool); !created {
		t.Fatalf("first set was not marked created: %v", meta(t, sets[1]))
	}
	if created, _ := meta(t, sets[0])["created"].(bool); created {
		t.Fatalf("replacement was marked created: %v", meta(t, sets[0]))
	}
	for action, want := range map[string]string{
		"secret.delete": "DATABASE_PASSWORD", "registry.create": "GHCR", "enrollment_token.create": "prod-01",
		"project.update": "Shop", "member.add": "teammate@example.com",
	} {
		got := find(entries, action)
		if len(got) != 1 || got[0].TargetName == nil || *got[0].TargetName != want {
			t.Errorf("%s: %+v, want target %q", action, got, want)
		}
	}
	if m := meta(t, find(entries, "member.add")[0]); m["role"] != "developer" {
		t.Errorf("member.add metadata %v", m)
	}
	// An update records what it changed, not just what it touched.
	if m := meta(t, find(entries, "project.update")[0]); m["name"] != "Shop Renamed" {
		t.Errorf("project.update metadata %v", m)
	}

	// No secret, password, or token reaches the audit log in any form.
	all, _ := json.Marshal(entries)
	for _, forbidden := range []string{secretValue, "another-secret-value", registryPassword, token} {
		if strings.Contains(string(all), forbidden) {
			t.Fatalf("the audit log contains %q:\n%s", forbidden, all)
		}
	}
}

func TestFailedRequestsAreNotRecorded(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	base := "/api/v1/projects/" + projectID.String()
	before := len(eventsOf(t, owner, projectID))

	owner.Do(http.MethodPut, base+"/secrets/lowercase", map[string]string{"value": "a-long-enough-value"}).RequireStatus(http.StatusUnprocessableEntity)
	owner.Do(http.MethodDelete, base+"/secrets/NOT_THERE", nil).RequireStatus(http.StatusNotFound)
	developer, developerID := env.NewUser("dev@example.com")
	env.AddMember(projectID, developerID, "developer")
	developer.Do(http.MethodPut, base+"/secrets/API_KEY", map[string]string{"value": "a-long-enough-value"}).RequireStatus(http.StatusForbidden)

	if after := len(eventsOf(t, owner, projectID)); after != before {
		t.Fatalf("failed requests added %d events: %+v", after-before, eventsOf(t, owner, projectID)[:after-before])
	}
}

func TestEnrollmentAndCreationsAreRecorded(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	appID := env.CreateApplication(owner, projectID, "API")
	server := env.EnrollAgent(owner, projectID, "prod-01")

	var staging struct{ ID uuid.UUID }
	owner.Do(http.MethodPost, "/api/v1/applications/"+appID.String()+"/environments", map[string]any{
		"name": "staging", "server_id": server.ServerID, "auto_deploy": true,
		"config": map[string]any{"container_port": 8080, "published_port": 18080},
	}).RequireStatus(http.StatusCreated).Decode(&staging)

	entries := eventsOf(t, owner, projectID)
	for action, want := range map[string]string{"server.enroll": "prod-01", "environment.create": "staging", "application.create": "API"} {
		got := find(entries, action)
		if len(got) != 1 || got[0].TargetName == nil || *got[0].TargetName != want {
			t.Errorf("%s: %+v, want target %q", action, got, want)
		}
	}
	if enroll := find(entries, "server.enroll"); len(enroll) == 1 && enroll[0].Actor != "agent" {
		t.Errorf("server.enroll actor %q, want agent", enroll[0].Actor)
	}
}

func TestSigningInIsRecordedUnderTheUsersEmail(t *testing.T) {
	env := apitest.New(t)
	_, userID := env.NewUser("owner@example.com")
	owner := env.SignIn("owner@example.com")
	projectID := owner.CreateProject("Shop")
	_ = userID

	// Login is installation-level (no project), so read it from the table.
	var actor string
	if err := env.Pool.QueryRow(t.Context(), `SELECT actor FROM audit_logs WHERE action = 'user.login' ORDER BY created_at DESC LIMIT 1`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if actor != "owner@example.com" {
		t.Fatalf("actor %q", actor)
	}
	_ = projectID
}
