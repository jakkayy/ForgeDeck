package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agent"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

func TestEnrollStoresIdentity(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	token := owner.IssueEnrollmentToken(owner.CreateProject("Infra"), "prod-01")
	state, err := agent.NewState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	identity, err := agent.Enroll(context.Background(), state, env.Server.URL, token, "0.1.0")
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	stored, err := state.Load()
	if err != nil || stored != identity || stored.ControlPlaneURL != env.Server.URL {
		t.Fatalf("stored %+v, %v", stored, err)
	}

	if _, err := agent.Enroll(context.Background(), state, env.Server.URL, token, "0.1.0"); err == nil {
		t.Fatal("re-enrolling over an existing identity succeeded")
	}
}

func TestEnrollWithUsedTokenIsUnauthorized(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	token := owner.IssueEnrollmentToken(owner.CreateProject("Infra"), "prod-01")

	first, _ := agent.NewState(t.TempDir())
	if _, err := agent.Enroll(context.Background(), first, env.Server.URL, token, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	second, _ := agent.NewState(t.TempDir())
	_, err := agent.Enroll(context.Background(), second, env.Server.URL, token, "0.1.0")
	if !errors.Is(err, worker.ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
}
