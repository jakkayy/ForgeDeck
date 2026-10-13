package app_test

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/app"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

// Who may call each project-scoped route. min is the lowest role allowed;
// the test proves both sides of that line: a role below it is refused (403),
// the minimum role is not refused, a non-member sees 404 (so membership
// cannot be probed), and a caller without a session gets 401.
type rbacRoute struct {
	route string // "METHOD pattern", as chi reports it
	min   authz.Role
	body  any
}

var (
	viewer, developer, admin, owner = authz.RoleViewer, authz.RoleDeveloper, authz.RoleAdmin, authz.RoleOwner

	projectRoutes = []rbacRoute{
		{"GET /api/v1/projects/{projectID}", viewer, nil},
		{"PATCH /api/v1/projects/{projectID}", admin, map[string]string{"name": "Renamed"}},
		{"DELETE /api/v1/projects/{projectID}", owner, map[string]string{"confirm": "Shop"}},
		{"GET /api/v1/projects/{projectID}/members", viewer, nil},
		{"POST /api/v1/projects/{projectID}/members", admin, map[string]string{"email": "nobody@example.com", "role": "viewer"}},
		{"DELETE /api/v1/projects/{projectID}/members/{userID}", admin, nil},

		{"GET /api/v1/projects/{projectID}/servers", viewer, nil},
		{"POST /api/v1/projects/{projectID}/servers/enrollment-tokens", admin, map[string]string{"server_name": "new-server"}},
		{"GET /api/v1/servers/{serverID}", viewer, nil},
		{"DELETE /api/v1/servers/{serverID}", admin, nil},
		{"GET /api/v1/servers/{serverID}/containers", viewer, nil},
		{"POST /api/v1/servers/{serverID}/containers/{containerID}/actions", developer, map[string]string{"action": "restart"}},
		{"POST /api/v1/servers/{serverID}/containers/{containerID}/logs", developer, map[string]any{"tail_lines": 10}},

		{"GET /api/v1/projects/{projectID}/git-connections", viewer, nil},
		{"POST /api/v1/projects/{projectID}/git-connections/github", admin, nil},
		{"POST /api/v1/projects/{projectID}/git-connections/gitlab", admin, map[string]string{"base_url": "", "token": ""}},
		{"DELETE /api/v1/git-connections/{connectionID}", admin, nil},
		{"GET /api/v1/git-connections/{connectionID}/repositories", admin, nil},

		{"GET /api/v1/projects/{projectID}/registries", viewer, nil},
		{"POST /api/v1/projects/{projectID}/registries", admin, map[string]string{"name": ""}},
		{"DELETE /api/v1/registries/{registryID}", admin, nil},

		{"GET /api/v1/projects/{projectID}/secrets", viewer, nil},
		{"PUT /api/v1/projects/{projectID}/secrets/{name}", admin, map[string]string{"value": "a-long-enough-value"}},
		{"DELETE /api/v1/projects/{projectID}/secrets/{name}", admin, nil},

		{"GET /api/v1/projects/{projectID}/applications", viewer, nil},
		{"POST /api/v1/projects/{projectID}/applications", admin, map[string]string{"name": ""}},
		{"GET /api/v1/applications/{applicationID}", viewer, nil},
		{"PATCH /api/v1/applications/{applicationID}", admin, map[string]string{"branch": "main"}},
		{"DELETE /api/v1/applications/{applicationID}", admin, nil},
		{"POST /api/v1/applications/{applicationID}/builds", developer, nil},

		{"GET /api/v1/applications/{applicationID}/environments", viewer, nil},
		{"POST /api/v1/applications/{applicationID}/environments", admin, map[string]string{"name": ""}},
		{"GET /api/v1/environments/{environmentID}", viewer, nil},
		{"PATCH /api/v1/environments/{environmentID}", admin, map[string]any{"auto_deploy": true}},
		{"DELETE /api/v1/environments/{environmentID}", admin, nil},
		{"GET /api/v1/environments/{environmentID}/deployments", viewer, nil},
		{"POST /api/v1/environments/{environmentID}/deployments", developer, map[string]string{"build_job_id": uuid.NewString()}},
		{"GET /api/v1/deployments/{deploymentID}", viewer, nil},
		{"GET /api/v1/environments/{environmentID}/releases", viewer, nil},
		{"POST /api/v1/releases/{releaseID}/rollback", developer, nil},

		{"GET /api/v1/jobs/{jobID}", viewer, nil},
		{"GET /api/v1/jobs/{jobID}/logs", viewer, nil},
		{"GET /api/v1/jobs/{jobID}/events", viewer, nil},
		{"POST /api/v1/jobs/{jobID}/cancel", developer, nil},

		{"GET /api/v1/projects/{projectID}/audit-logs", admin, nil},
	}
)

// Routes that are not scoped to one project, with why that is fine. Everything
// else the router serves must be in projectRoutes.
var notProjectScoped = map[string]string{
	"GET /api/v1/projects":         "lists only the caller's own projects",
	"POST /api/v1/projects":        "any signed-in user may create a project; they become its owner",
	"GET /api/v1/servers":          "lists only servers of the caller's projects",
	"GET /api/v1/jobs":             "lists only jobs of the caller's projects",
	"GET /api/v1/auth/me":          "the caller's own user",
	"POST /api/v1/auth/login":      "public by nature; rate limited",
	"POST /api/v1/auth/logout":     "ends the caller's own session",
	"GET /api/v1/git/github/setup": "GitHub's redirect; verified by a sealed state and OAuth",
	"GET /healthz":                 "public liveness check",
	"GET /*":                       "the embedded web UI",
}

// machinePrefixes are authenticated by their own credentials (Agent, Runner)
// or by a signature (webhooks), not by a user session or a project role.
var machinePrefixes = []string{"/api/v1/agent/", "/api/v1/runner/", "/api/v1/webhooks/"}

func TestEveryRouteIsClassifiedForAuthorization(t *testing.T) {
	handler := app.NewRouter(t.Context(), config.Config{}, testdb.New(t), slog.New(slog.DiscardHandler))
	routes, ok := handler.(chi.Routes)
	if !ok {
		t.Fatal("the router is not a chi.Routes")
	}
	covered := map[string]bool{}
	for _, r := range projectRoutes {
		covered[r.route] = true
	}
	seen := map[string]bool{}
	_ = chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		key := method + " " + route
		seen[key] = true
		_, public := notProjectScoped[key]
		machine := false
		for _, prefix := range machinePrefixes {
			machine = machine || strings.HasPrefix(route, prefix)
		}
		if method != http.MethodHead && !covered[key] && !public && !machine {
			t.Errorf("%s is served but is neither in projectRoutes (with a minimum role) nor in notProjectScoped", key)
		}
		return nil
	})
	for _, r := range projectRoutes {
		if !seen[r.route] {
			t.Errorf("projectRoutes has %s, which is not a route", r.route)
		}
	}
	for key := range notProjectScoped {
		if !seen[key] {
			t.Errorf("notProjectScoped has %s, which is not a route", key)
		}
	}
}

// world is a project with one member of each role, a non-member, and one of
// every kind of resource a route can name.
type world struct {
	env                             *apitest.Env
	owner, admin, developer, viewer *apitest.Client
	outsider, anonymous             *apitest.Client
	vars                            map[string]string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	env := apitest.New(t)
	w := &world{env: env, anonymous: env.Anonymous()}
	var viewerID uuid.UUID
	w.owner, _ = env.NewUser("owner@example.com")
	projectID := w.owner.CreateProject("Shop")
	member := func(email, role string) (*apitest.Client, uuid.UUID) {
		client, id := env.NewUser(email)
		env.AddMember(projectID, id, role)
		return client, id
	}
	w.admin, _ = member("admin@example.com", "admin")
	w.developer, _ = member("developer@example.com", "developer")
	w.viewer, viewerID = member("viewer@example.com", "viewer")
	w.outsider, _ = env.NewUser("outsider@example.com")

	appID := env.CreateApplication(w.owner, projectID, "API")
	server := env.EnrollAgent(w.owner, projectID, "prod-01")
	const containerID = "aaaaaaaaaaaa1111"
	server.ReportContainers(t, containerID)

	ctx := context.Background()
	var connectionID, registryID, environmentID uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(env.Pool.QueryRow(ctx, `SELECT id FROM git_connections WHERE project_id = $1`, projectID).Scan(&connectionID))
	var registry struct{ ID uuid.UUID }
	w.owner.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/registries",
		map[string]string{"name": "GHCR", "host": "ghcr.io", "username": "bot", "password": "registry-password"}).
		RequireStatus(http.StatusCreated).Decode(&registry)
	registryID = registry.ID
	w.owner.Do(http.MethodPut, "/api/v1/projects/"+projectID.String()+"/secrets/API_KEY", map[string]string{"value": "a-long-enough-value"}).
		RequireStatus(http.StatusCreated)
	var environment struct{ ID uuid.UUID }
	w.owner.Do(http.MethodPost, "/api/v1/applications/"+appID.String()+"/environments", map[string]any{
		"name": "staging", "server_id": server.ServerID, "config": map[string]any{"container_port": 8080, "published_port": 18080},
	}).RequireStatus(http.StatusCreated).Decode(&environment)
	environmentID = environment.ID

	// A release (with its deployment) and a queued job.
	config := `{"container_port":8080,"published_port":18080,"bind_address":"127.0.0.1","env":{},"secrets":[],"volumes":[],"restart":"unless-stopped","health_check":{"type":"tcp","timeout_seconds":60,"interval_seconds":2}}`
	ref := "ghcr.io/acme/shop@sha256:" + strings.Repeat("a", 64)
	var deploymentID, releaseID uuid.UUID
	must(env.Pool.QueryRow(ctx, `INSERT INTO deployments (environment_id, commit_sha, image_ref, config, status, source_release_id)
		VALUES ($1, $2, $3, $4::jsonb, 'succeeded', NULL) RETURNING id`, environmentID, strings.Repeat("1", 40), ref, config).Scan(&deploymentID))
	must(env.Pool.QueryRow(ctx, `INSERT INTO releases (application_id, environment_id, deployment_id, commit_sha, image_ref, config)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb) RETURNING id`, appID, environmentID, deploymentID, strings.Repeat("1", 40), ref, config).Scan(&releaseID))
	job, err := jobs.NewService(env.Pool, authz.New(store.New(env.Pool)), time.Now).Enqueue(ctx, jobs.NewJob{
		ProjectID: projectID, ApplicationID: &appID, Type: agentproto.JobBuild, WorkerKind: jobs.WorkerRunner, Payload: map[string]string{}})
	must(err)

	w.vars = map[string]string{
		"{projectID}": projectID.String(), "{userID}": viewerID.String(), "{serverID}": server.ServerID.String(),
		"{containerID}": containerID, "{connectionID}": connectionID.String(), "{registryID}": registryID.String(),
		"{name}": "API_KEY", "{applicationID}": appID.String(), "{environmentID}": environmentID.String(),
		"{deploymentID}": deploymentID.String(), "{releaseID}": releaseID.String(), "{jobID}": job.ID.String(),
	}
	return w
}

func (w *world) path(pattern string) string {
	path := pattern
	for name, value := range w.vars {
		path = strings.ReplaceAll(path, name, value)
	}
	return path
}

func (w *world) client(role authz.Role) *apitest.Client {
	return map[authz.Role]*apitest.Client{viewer: w.viewer, developer: w.developer, admin: w.admin, owner: w.owner}[role]
}

// below is the highest role that is not allowed to call a route with minimum min.
func below(min authz.Role) (authz.Role, bool) {
	switch min {
	case developer:
		return viewer, true
	case admin:
		return developer, true
	case owner:
		return admin, true
	default:
		return "", false
	}
}

func (w *world) call(client *apitest.Client, method, path string, body any) int {
	if method == http.MethodGet && strings.HasSuffix(path, "/events") {
		// An event stream never ends; the status line is all that matters.
		resp := client.Stream(path, nil)
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	return client.Do(method, path, body).Status
}

func TestRoleChecksOnEveryProjectRoute(t *testing.T) {
	// Reads and idempotent probes share one world; routes that change or
	// delete something get a fresh one, so one route cannot remove what the
	// next one needs.
	shared := newWorld(t)
	for _, r := range projectRoutes {
		t.Run(r.route, func(t *testing.T) {
			method, pattern, _ := strings.Cut(r.route, " ")
			w := shared
			if method != http.MethodGet {
				w = newWorld(t)
			}
			path := w.path(pattern)

			if got := w.call(w.anonymous, method, path, r.body); got != http.StatusUnauthorized {
				t.Errorf("without a session: %d, want 401", got)
			}
			if got := w.call(w.outsider, method, path, r.body); got != http.StatusNotFound {
				t.Errorf("as a non-member: %d, want 404 (membership must not be probeable)", got)
			}
			if lower, ok := below(r.min); ok {
				if got := w.call(w.client(lower), method, path, r.body); got != http.StatusForbidden {
					t.Errorf("as %s, below the minimum %s: %d, want 403", lower, r.min, got)
				}
			}
			if got := w.call(w.client(r.min), method, path, r.body); got == http.StatusUnauthorized || got == http.StatusForbidden || got == http.StatusNotFound {
				t.Errorf("as %s, the minimum role: %d, want the request to be allowed", r.min, got)
			}
		})
	}
}
