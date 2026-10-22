package app_test

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"strconv"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/config"
)

// behindProxy trusts this machine as a reverse proxy so tests can vary X-Forwarded-For.
func behindProxy(cfg *config.Config) {
	cfg.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
}

func asClient(ip string) map[string]string {
	return map[string]string{"X-Forwarded-For": ip, "Content-Type": "application/json"}
}

func TestEnrollmentGuessingIsLimitedPerClientAddress(t *testing.T) {
	env := apitest.New(t, behindProxy)
	anon := env.Anonymous()
	guess := []byte(`{"token":"fde_wrong","agent_version":"x","protocol_version":1}`)

	var statuses []int
	for range 12 {
		statuses = append(statuses, anon.DoRaw(http.MethodPost, "/api/v1/agent/enroll", guess, asClient("198.51.100.10")).Status)
	}
	if statuses[0] != http.StatusUnauthorized || statuses[9] != http.StatusUnauthorized || statuses[10] != http.StatusTooManyRequests {
		t.Fatalf("statuses %v: want 401 for the first 10 guesses, then 429", statuses)
	}
	resp := anon.DoRaw(http.MethodPost, "/api/v1/agent/enroll", guess, asClient("198.51.100.10"))
	if resp.Status != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("limited response %d, Retry-After %q", resp.Status, resp.Header.Get("Retry-After"))
	}

	if got := anon.DoRaw(http.MethodPost, "/api/v1/agent/enroll", guess, asClient("198.51.100.11")).Status; got != http.StatusUnauthorized {
		t.Fatalf("a different client got %d, want a plain 401", got)
	}
}

func TestOneClientsGuessesDoNotLimitRealAgents(t *testing.T) {
	env := apitest.New(t, behindProxy)
	owner, _ := env.NewUser("owner@example.com")
	agent := env.EnrollAgent(owner, owner.CreateProject("Infra"), "prod-01")
	beat, _ := jsonBody(agentproto.HeartbeatRequest{AgentVersion: "t", ProtocolVersion: agentproto.ProtocolVersion})

	for i := range 60 {
		if got := agent.DoRaw(http.MethodPost, "/api/v1/agent/heartbeat", beat, asClient("203.0.113.5")).Status; got != http.StatusOK {
			t.Fatalf("heartbeat %d: %d", i, got)
		}
	}

	bad := env.Anonymous()
	var last int
	for range 25 {
		last = bad.DoRaw(http.MethodPost, "/api/v1/agent/heartbeat", beat, asClient("198.51.100.20")).Status
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("after 25 bad credentials the status is %d, want 429", last)
	}
	if got := agent.DoRaw(http.MethodPost, "/api/v1/agent/heartbeat", beat, asClient("203.0.113.5")).Status; got != http.StatusOK {
		t.Fatalf("the real Agent got %d", got)
	}
}

func TestBadWebhookSignaturesAreThrottledPerClientAddress(t *testing.T) {
	env := apitest.New(t, behindProxy)
	anon := env.Anonymous()
	path := "/api/v1/webhooks/gitlab/00000000-0000-0000-0000-000000000001"
	headers := asClient("198.51.100.30")
	headers["X-Gitlab-Token"], headers["X-Gitlab-Event"] = "guess", "Push Hook"

	var last int
	for range 25 {
		last = anon.DoRaw(http.MethodPost, path, []byte(`{}`), headers).Status
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("status %d after 25 bad tokens, want 429", last)
	}
}

func TestSensitiveUserActionsHaveTighterLimits(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	path := "/api/v1/projects/" + projectID.String() + "/members"

	// looking up emails one by one would reveal which have accounts.
	var statuses []int
	for i := range 12 {
		statuses = append(statuses, owner.Do(http.MethodPost, path, map[string]string{"email": "nobody" + strconv.Itoa(i) + "@example.com", "role": "viewer"}).Status)
	}
	for i, status := range statuses {
		want := http.StatusUnprocessableEntity
		if i >= 10 {
			want = http.StatusTooManyRequests
		}
		if status != want && (i >= 10 || status != http.StatusNotFound) {
			t.Fatalf("attempt %d: %d, want %d (statuses %v)", i, status, want, statuses)
		}
	}
	other, _ := env.NewUser("other@example.com")
	otherProject := other.CreateProject("Other")
	other.Do(http.MethodPost, "/api/v1/projects/"+otherProject.String()+"/members", map[string]string{"email": "x@example.com", "role": "viewer"}).
		RequireStatus(http.StatusUnprocessableEntity)
}

func TestUsersMayChangeThingsFastButNotWithoutLimit(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	path := "/api/v1/projects/" + projectID.String()

	var limited int
	for i := range 70 {
		resp := owner.Do(http.MethodPatch, path, map[string]string{"name": "Shop " + strconv.Itoa(i)})
		if resp.Status == http.StatusTooManyRequests {
			limited++
			if resp.Header.Get("Retry-After") == "" {
				t.Fatal("429 without Retry-After")
			}
		} else if resp.Status != http.StatusOK {
			t.Fatalf("change %d: %d", i, resp.Status)
		}
	}
	if limited < 5 {
		t.Fatalf("%d of 70 rapid changes were limited, want most of those past the burst of 60", limited)
	}
	for range 100 {
		owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK)
	}
}

func jsonBody(v any) ([]byte, error) { return json.Marshal(v) }
