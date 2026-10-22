package servers_test

import (
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

// TestServersAreInvisibleAcrossProjects: another project's admin gets 404, not 403, everywhere.
func TestServersAreInvisibleAcrossProjects(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	intruder, _ := env.NewUser("intruder@example.com")
	victimProject := owner.CreateProject("Victim")
	intruder.CreateProject("Intruder's own")
	agent := env.EnrollAgent(owner, victimProject, "prod-01")
	agent.ReportContainers(t, apiContainer)

	server := "/api/v1/servers/" + agent.ServerID.String()
	container := server + "/containers/" + apiContainer
	attempts := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, server, nil},
		{http.MethodGet, server + "/containers", nil},
		{http.MethodPost, container + "/actions", map[string]string{"action": "stop"}},
		{http.MethodPost, container + "/logs", map[string]any{}},
		{http.MethodDelete, server, nil},
		{http.MethodGet, "/api/v1/projects/" + victimProject.String() + "/servers", nil},
		{http.MethodPost, "/api/v1/projects/" + victimProject.String() + "/servers/enrollment-tokens", map[string]string{"server_name": "x"}},
	}
	for _, a := range attempts {
		if resp := intruder.Do(a.method, a.path, a.body); resp.Status != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404", a.method, a.path, resp.Status)
		}
	}

	var visible httpx.List[servers.Server]
	intruder.Do(http.MethodGet, "/api/v1/servers", nil).RequireStatus(http.StatusOK).Decode(&visible)
	if len(visible.Items) != 0 {
		t.Fatalf("intruder lists %d foreign servers", len(visible.Items))
	}

	owner.Do(http.MethodGet, server, nil).RequireStatus(http.StatusOK)
	var jobs int
	if err := env.Pool.QueryRow(t.Context(), "SELECT count(*) FROM jobs").Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("intruder created %d jobs (err %v)", jobs, err)
	}
}
