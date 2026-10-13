package agentapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentapi"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/servers"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

func TestRequireAgent(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	agent := env.EnrollAgent(owner, projectID, "prod-01")

	svc := servers.NewService(env.Pool, authz.New(store.New(env.Pool)), nil, time.Now)
	router := chi.NewRouter()
	router.Use(agentapi.NewHandler(context.Background(), svc, nil, nil).RequireAgent)
	router.Get("/whoami", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(agentapi.IdentityFrom(r.Context()))
	})

	call := func(authorization string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := call("Bearer " + agent.Credential)
	var identity servers.AgentIdentity
	if err := json.Unmarshal(rec.Body.Bytes(), &identity); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if identity.AgentID != agent.AgentID || identity.ServerID != agent.ServerID || identity.ProjectID != projectID {
		t.Fatalf("identity %+v", identity)
	}

	for _, header := range []string{"", "Bearer ", "Bearer fda_forged", "Basic " + agent.Credential} {
		if code := call(header).Code; code != http.StatusUnauthorized {
			t.Errorf("Authorization %q: status %d", header, code)
		}
	}
}
