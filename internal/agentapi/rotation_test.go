package agentapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

func beat(t *testing.T, client *apitest.Client) (int, agentproto.HeartbeatResponse) {
	t.Helper()
	resp := client.Do(http.MethodPost, "/api/v1/agent/heartbeat", agentproto.HeartbeatRequest{AgentVersion: "t", ProtocolVersion: agentproto.ProtocolVersion})
	var out agentproto.HeartbeatResponse
	if resp.Status == http.StatusOK {
		resp.Decode(&out)
	}
	return resp.Status, out
}

func rotate(client *apitest.Client) *apitest.Response {
	return client.Do(http.MethodPost, "/api/v1/agent/rotate-credential", map[string]string{})
}

func withCredential(env *apitest.Env, credential string) *apitest.Client {
	return env.AgentClient(credential)
}

func TestRotatedCredentialWorksAndTheOldOneOnlyBrieflyAndAsksToRotateAgain(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	agent := env.EnrollAgent(owner, owner.CreateProject("Infra"), "prod-01")

	status, hb := beat(t, agent.Client)
	if status != http.StatusOK || hb.RotateCredential {
		t.Fatalf("a fresh credential: %d, rotate=%v", status, hb.RotateCredential)
	}

	var rotated agentproto.RotateResponse
	resp := rotate(agent.Client).RequireStatus(http.StatusOK)
	resp.Decode(&rotated)
	if rotated.Credential == "" || rotated.Credential == agent.Credential || rotated.ServerID != agent.ServerID ||
		resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("rotation response %+v, Cache-Control %q", rotated, resp.Header.Get("Cache-Control"))
	}
	fresh := withCredential(env, rotated.Credential)

	// The new credential is the current one: nothing more to do.
	if status, hb := beat(t, fresh); status != http.StatusOK || hb.RotateCredential {
		t.Fatalf("new credential: %d, rotate=%v", status, hb.RotateCredential)
	}
	// The old one still works during the grace period, and says "rotate again":
	// an Agent that never saved the new one converges instead of being locked out.
	if status, hb := beat(t, agent.Client); status != http.StatusOK || !hb.RotateCredential {
		t.Fatalf("old credential in grace: %d, rotate=%v", status, hb.RotateCredential)
	}

	// Once the grace period is over the old credential is dead.
	if _, err := env.Pool.Exec(context.Background(), `UPDATE agents SET previous_expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if status, _ := beat(t, agent.Client); status != http.StatusUnauthorized {
		t.Fatalf("old credential after the grace period: %d, want 401", status)
	}
	if status, _ := beat(t, fresh); status != http.StatusOK {
		t.Fatalf("new credential after the grace period: %d", status)
	}
}

func TestAnAgentThatLostTheNewCredentialCanRotateAgain(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	agent := env.EnrollAgent(owner, owner.CreateProject("Infra"), "prod-01")

	var first, second agentproto.RotateResponse
	rotate(agent.Client).RequireStatus(http.StatusOK).Decode(&first)
	// The response was lost: the Agent still holds the original and rotates with it.
	rotate(agent.Client).RequireStatus(http.StatusOK).Decode(&second)
	if first.Credential == second.Credential {
		t.Fatal("two rotations returned the same credential")
	}
	if status, _ := beat(t, withCredential(env, second.Credential)); status != http.StatusOK {
		t.Fatalf("the latest credential: %d", status)
	}
	// The credential issued by the abandoned first rotation no longer works:
	// only the latest and the one presented last are valid.
	if status, _ := beat(t, withCredential(env, first.Credential)); status != http.StatusUnauthorized {
		t.Fatalf("a superseded credential: %d, want 401", status)
	}
}

func TestAdminsCanRequestRotationAndTheAgentIsToldOnce(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	agent := env.EnrollAgent(owner, projectID, "prod-01")
	request := "/api/v1/servers/" + agent.ServerID.String() + "/credential-rotation"

	owner.Do(http.MethodPost, request, nil).RequireStatus(http.StatusAccepted)
	if _, hb := beat(t, agent.Client); !hb.RotateCredential {
		t.Fatal("the Agent was not asked to rotate after an admin requested it")
	}
	// Still asked until it does it.
	if _, hb := beat(t, agent.Client); !hb.RotateCredential {
		t.Fatal("the request was forgotten before the Agent acted")
	}
	var rotated agentproto.RotateResponse
	rotate(agent.Client).RequireStatus(http.StatusOK).Decode(&rotated)
	if _, hb := beat(t, withCredential(env, rotated.Credential)); hb.RotateCredential {
		t.Fatal("the request was not cleared by the rotation")
	}
}

func TestOldCredentialsAreRotatedWithoutAnAdminAsking(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	agent := env.EnrollAgent(owner, owner.CreateProject("Infra"), "prod-01")
	if _, err := env.Pool.Exec(context.Background(), `UPDATE agents SET credential_rotated_at = now() - interval '91 days'`); err != nil {
		t.Fatal(err)
	}
	if _, hb := beat(t, agent.Client); !hb.RotateCredential {
		t.Fatal("a 91-day-old credential was not rotated")
	}
}

func TestRotationNeedsAValidCredential(t *testing.T) {
	env := apitest.New(t)
	rotate(withCredential(env, "fda_not-a-credential")).RequireStatus(http.StatusUnauthorized)
	rotate(env.Anonymous()).RequireStatus(http.StatusUnauthorized)
}
