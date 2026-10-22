package agent_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agent"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

func identityOf(t *testing.T, enrolled agentproto.EnrollResponse, url string) agent.Identity {
	t.Helper()
	return agent.Identity{ControlPlaneURL: url, AgentID: enrolled.AgentID, ServerID: enrolled.ServerID, Credential: enrolled.Credential}
}

func TestAgentRotatesItsCredentialWhenAskedAndKeepsWorking(t *testing.T) {
	env, owner, enrolled, _ := enrolledClient(t)
	dir := t.TempDir()
	state, err := agent.NewState(dir)
	if err != nil {
		t.Fatal(err)
	}
	identity := identityOf(t, enrolled.EnrollResponse, env.Server.URL)
	if err := state.Save(identity); err != nil {
		t.Fatal(err)
	}
	client := worker.NewClient(env.Server.URL, worker.AgentAPI, enrolled.Credential)
	rotator := agent.NewRotator(client, state, identity, slog.New(slog.DiscardHandler))

	owner.Do(http.MethodPost, "/api/v1/servers/"+enrolled.ServerID.String()+"/credential-rotation", nil).RequireStatus(http.StatusAccepted)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- worker.NewHeartbeater(client, "0.5.0", dockerUp, slog.New(slog.DiscardHandler)).OnRotate(rotator.Rotate).Run(ctx)
	}()

	var saved agent.Identity
	deadline := time.Now().Add(5 * time.Second)
	for {
		saved, err = state.Load()
		if err == nil && saved.Credential != enrolled.Credential {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the saved credential never changed: %+v, %v", saved, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasPrefix(saved.Credential, agentproto.CredentialPrefix) || saved.ServerID != enrolled.ServerID {
		t.Fatalf("saved identity %+v", saved)
	}

	info, err := os.Stat(filepath.Join(dir, "identity.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("identity file: %v, %v", info, err)
	}

	if _, err := env.Pool.Exec(context.Background(), `UPDATE agents SET previous_expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Heartbeat(context.Background(), agentproto.HeartbeatRequest{AgentVersion: "t", ProtocolVersion: agentproto.ProtocolVersion}); err != nil {
		t.Fatalf("after the old credential expired: %v", err)
	}
	stale := worker.NewClient(env.Server.URL, worker.AgentAPI, enrolled.Credential)
	if _, err := stale.Heartbeat(context.Background(), agentproto.HeartbeatRequest{AgentVersion: "t", ProtocolVersion: agentproto.ProtocolVersion}); !errors.Is(err, worker.ErrUnauthorized) {
		t.Fatalf("the replaced credential: %v, want unauthorized", err)
	}
}

func TestFailedSaveKeepsTheCurrentCredentialAndRecoversAtTheNextAttempt(t *testing.T) {
	env, _, enrolled, _ := enrolledClient(t)
	dir := t.TempDir()
	state, err := agent.NewState(dir)
	if err != nil {
		t.Fatal(err)
	}
	identity := identityOf(t, enrolled.EnrollResponse, env.Server.URL)
	client := worker.NewClient(env.Server.URL, worker.AgentAPI, enrolled.Credential)
	rotator := agent.NewRotator(client, state, identity, slog.New(slog.DiscardHandler))

	// the disk refuses the write: the Agent must not switch to a credential it could not save.
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // simulating a read-only state directory
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if err := rotator.Rotate(context.Background()); err == nil {
		t.Fatal("Rotate succeeded although the credential could not be saved")
	}
	if _, err := client.Heartbeat(context.Background(), agentproto.HeartbeatRequest{AgentVersion: "t", ProtocolVersion: agentproto.ProtocolVersion}); err != nil {
		t.Fatalf("the Agent no longer works with the credential it kept: %v", err)
	}

	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // directories need the execute bit
		t.Fatal(err)
	}
	if err := rotator.Rotate(context.Background()); err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	saved, err := state.Load()
	if err != nil || saved.Credential == enrolled.Credential {
		t.Fatalf("saved %+v, %v", saved, err)
	}
}

func dockerUp(context.Context) (agentproto.Docker, *agentproto.Resources) {
	return agentproto.Docker{Available: true, Version: "27.3.1"}, nil
}
