package docker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

const testImage = "alpine:3.20"

// TestEngineAgainstDockerDaemon exercises every allowlisted operation on a
// throwaway container. It needs a Docker daemon and is opt-in:
// FORGEDECK_TEST_DOCKER=1 go test ./internal/agent/docker/
func TestEngineAgainstDockerDaemon(t *testing.T) {
	if os.Getenv("FORGEDECK_TEST_DOCKER") != "1" {
		t.Skip("set FORGEDECK_TEST_DOCKER=1 to run against a Docker daemon")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	if status := engine.Status(ctx); !status.Available {
		t.Fatal("Docker daemon not available")
	}
	id := createTestContainer(ctx, t, engine.cli)

	if err := engine.Start(ctx, id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := engine.Start(ctx, id); err != nil {
		t.Fatalf("Start on a running container should succeed: %v", err)
	}
	listed := findContainer(ctx, t, engine, id)
	if listed.State != "running" || !strings.HasPrefix(listed.Name, "forgedeck-test-") {
		t.Fatalf("listed %+v", listed)
	}
	if listed.Labels["forgedeck.test"] != "1" || listed.Labels["vendor.secret"] != "" {
		t.Fatalf("labels not filtered: %v", listed.Labels)
	}

	var stdout, stderr bytes.Buffer
	waitForOutput(ctx, t, engine, id, &stdout, &stderr)
	if !strings.Contains(stdout.String(), "to-stdout") || !strings.Contains(stderr.String(), "to-stderr") {
		t.Fatalf("logs not demultiplexed: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	if err := engine.Restart(ctx, id); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if err := engine.Stop(ctx, id); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := engine.Stop(ctx, id); err != nil {
		t.Fatalf("Stop on a stopped container should succeed: %v", err)
	}
	if state := findContainer(ctx, t, engine, id).State; state != "exited" {
		t.Fatalf("state after stop: %s", state)
	}

	if _, err := engine.cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Start(ctx, id); !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("Start on a removed container: got %v, want ErrContainerNotFound", err)
	}
}

func createTestContainer(ctx context.Context, t *testing.T, cli *client.Client) string {
	t.Helper()
	pull, err := cli.ImagePull(ctx, testImage, client.ImagePullOptions{})
	if err != nil {
		t.Fatalf("pull %s: %v", testImage, err)
	}
	if err := pull.Wait(ctx); err != nil {
		t.Fatalf("pull %s: %v", testImage, err)
	}

	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: "forgedeck-test-" + hex.EncodeToString(suffix),
		Config: &container.Config{
			Image:  testImage,
			Labels: map[string]string{"forgedeck.test": "1", "vendor.secret": "hunter2"},
			// Exit promptly on SIGTERM so stop/restart do not wait for the kill timeout.
			Cmd: []string{"sh", "-c", `trap 'exit 0' TERM; echo to-stdout; echo to-stderr >&2; while :; do sleep 1; done`},
		},
	})
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	t.Cleanup(func() {
		_, _ = cli.ContainerRemove(context.Background(), created.ID, client.ContainerRemoveOptions{Force: true})
	})
	return created.ID
}

func findContainer(ctx context.Context, t *testing.T, engine *Engine, id string) agentproto.Container {
	t.Helper()
	containers, err := engine.ListContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range containers {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("container %s not listed", id)
	return agentproto.Container{}
}

func waitForOutput(ctx context.Context, t *testing.T, engine *Engine, id string, stdout, stderr *bytes.Buffer) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		stdout.Reset()
		stderr.Reset()
		if err := engine.Logs(ctx, id, LogOptions{TailLines: 10}, stdout, stderr); err != nil {
			t.Fatalf("Logs: %v", err)
		}
		if stdout.Len() > 0 && stderr.Len() > 0 {
			return
		}
	}
}
