package audit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/audit"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

func recorder(env *apitest.Env) *audit.Recorder {
	return audit.NewRecorder(store.New(env.Pool), slog.New(slog.DiscardHandler))
}

func list(client *apitest.Client, projectID uuid.UUID, query string) *apitest.Response {
	return client.Do(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/audit-logs"+query, nil)
}

func TestRecordedEventsAreListedNewestFirstWithTheActorsEmail(t *testing.T) {
	env := apitest.New(t)
	owner, ownerID := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	rec := recorder(env)

	rec.Record(context.Background(), audit.Event{ProjectID: &projectID, ActorUserID: &ownerID, Action: "secret.set",
		TargetType: "secret", TargetName: "DATABASE_PASSWORD", Metadata: map[string]any{"version": 2}})
	rec.Record(context.Background(), audit.Event{ProjectID: &projectID, Actor: audit.ActorAgent, Action: "agent.enroll", TargetType: "server", TargetID: "s-1"})

	var page httpx.List[audit.Entry]
	list(owner, projectID, "").RequireStatus(http.StatusOK).Decode(&page)
	if len(page.Items) < 2 {
		t.Fatalf("items %+v", page.Items)
	}
	newest, older := page.Items[0], page.Items[1]
	if newest.Action != "agent.enroll" || newest.Actor != "agent" || newest.TargetID == nil || *newest.TargetID != "s-1" {
		t.Fatalf("newest %+v", newest)
	}
	var metadata map[string]any
	_ = json.Unmarshal(older.Metadata, &metadata)
	if older.Action != "secret.set" || older.Actor != "owner@example.com" || older.TargetName == nil ||
		*older.TargetName != "DATABASE_PASSWORD" || metadata["version"] != float64(2) {
		t.Fatalf("older %+v %v", older, metadata)
	}
}

func TestCredentialLikeMetadataIsMasked(t *testing.T) {
	env := apitest.New(t)
	owner, ownerID := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	recorder(env).Record(context.Background(), audit.Event{ProjectID: &projectID, ActorUserID: &ownerID, Action: "registry.create",
		TargetType: "registry", Metadata: map[string]any{"host": "ghcr.io", "password": "hunter2hunter2", "api_token": "tok", "value": "v"}})

	resp := list(owner, projectID, "?action=registry.create").RequireStatus(http.StatusOK)
	body := string(resp.Body)
	if strings.Contains(body, "hunter2") || strings.Contains(body, `"tok"`) || !strings.Contains(body, "ghcr.io") {
		t.Fatalf("body %s", body)
	}
}

func TestListPaginatesAndFilters(t *testing.T) {
	env := apitest.New(t)
	owner, ownerID := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	rec := recorder(env)
	for i := range 5 {
		action := "secret.set"
		if i%2 == 1 {
			action = "secret.delete"
		}
		rec.Record(context.Background(), audit.Event{ProjectID: &projectID, ActorUserID: &ownerID, Action: action,
			TargetType: "secret", TargetName: fmt.Sprintf("S%d", i)})
	}

	seen := map[uuid.UUID]bool{}
	cursor := ""
	for {
		var page httpx.List[audit.Entry]
		list(owner, projectID, "?limit=2&cursor="+cursor).RequireStatus(http.StatusOK).Decode(&page)
		for _, e := range page.Items {
			if seen[e.ID] {
				t.Fatalf("event %s listed twice", e.ID)
			}
			seen[e.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	// 5 recorded plus the project's own creation, if it is audited.
	if len(seen) < 5 {
		t.Fatalf("saw %d events", len(seen))
	}

	var deletes httpx.List[audit.Entry]
	list(owner, projectID, "?action=secret.delete").RequireStatus(http.StatusOK).Decode(&deletes)
	if len(deletes.Items) != 2 {
		t.Fatalf("filtered %+v", deletes.Items)
	}
	list(owner, projectID, "?cursor=not-a-cursor").RequireStatus(http.StatusBadRequest)
}

func TestOnlyAdminsReadTheAuditLogOfTheirOwnProject(t *testing.T) {
	env := apitest.New(t)
	owner, ownerID := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	other := owner.CreateProject("Other")
	rec := recorder(env)
	rec.Record(context.Background(), audit.Event{ProjectID: &other, ActorUserID: &ownerID, Action: "secret.set", TargetType: "secret", TargetName: "ELSEWHERE"})

	developer, developerID := env.NewUser("dev@example.com")
	env.AddMember(projectID, developerID, "developer")
	admin, adminID := env.NewUser("admin@example.com")
	env.AddMember(projectID, adminID, "admin")
	outsider, _ := env.NewUser("outsider@example.com")

	list(developer, projectID, "").RequireStatus(http.StatusForbidden)
	list(outsider, projectID, "").RequireStatus(http.StatusNotFound)
	resp := list(admin, projectID, "").RequireStatus(http.StatusOK)
	if strings.Contains(string(resp.Body), "ELSEWHERE") {
		t.Fatalf("another project's event leaked: %s", resp.Body)
	}
}
