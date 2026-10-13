package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agent/compose"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/dockertest"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// pushImage builds an image from dockerfile, pushes it to the registry at
// host, and returns its digest reference.
func pushImage(t *testing.T, host, name, dockerfile string) string {
	t.Helper()
	tag := host + "/e2e/" + name + ":1"
	dockertest.RemoveImage(t, tag)
	build := exec.CommandContext(t.Context(), "docker", "build", "--quiet", "--tag", tag, "-") //nolint:gosec // test
	build.Stdin = strings.NewReader(dockerfile)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), "docker", "push", tag).CombinedOutput(); err != nil { //nolint:gosec // test
		t.Fatalf("push: %v\n%s", err, out)
	}
	out, err := exec.CommandContext(t.Context(), "docker", "image", "inspect", "--format", "{{index .RepoDigests 0}}", tag).Output() //nolint:gosec // test
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// webServer answers "deployed" on port 8080; silentServer runs but never listens.
const (
	webServer    = "FROM busybox:1.37\nRUN mkdir /www && echo deployed > /www/index.html\nCMD [\"httpd\", \"-f\", \"-p\", \"8080\", \"-h\", \"/www\"]\n"
	silentServer = "FROM busybox:1.37\nCMD [\"sleep\", \"3600\"]\n"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func httpBody(t *testing.T, port int) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return strings.TrimSpace(string(body))
		}
		if time.Now().After(deadline) {
			t.Fatalf("nothing answered on port %d", port)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestDeployWithRealDockerCompose(t *testing.T) {
	dockertest.Require(t)
	image := pushImage(t, dockertest.StartRegistry(t), "web", webServer)
	d := DeployImageHandler(filepath.Join(t.TempDir(), "deployments"))

	environmentID := uuid.New()
	port, container := freePort(t), 8080
	deploy := func(env map[string]string) agentproto.DeployResult {
		t.Helper()
		raw, _ := json.Marshal(agentproto.DeployImagePayload{
			DeploymentID: uuid.New(), EnvironmentID: environmentID, ApplicationID: uuid.New(), CommitSHA: "abc", ImageRef: image,
			Config: servicecfg.Config{ContainerPort: &container, PublishedPort: &port, Env: env,
				Volumes: []servicecfg.Volume{{Name: "data", Path: "/data"}}},
		})
		log := &logRecorder{}
		result, err := d(context.Background(), &agentproto.Job{Type: agentproto.JobDeployImage, Payload: raw}, log)
		if err != nil {
			t.Fatalf("deploy: %v\n%s", err, log)
		}
		return result.(agentproto.DeployResult)
	}
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), "docker", "compose", "--project-name", compose.ProjectName(environmentID), "down", "--volumes").Run() //nolint:gosec // test
	})

	first := deploy(map[string]string{"A": "1"})
	if body := httpBody(t, port); body != "deployed" {
		t.Fatalf("service answered %q", body)
	}
	labels, err := exec.CommandContext(t.Context(), "docker", "inspect", "--format", "{{index .Config.Labels \"forgedeck.managed\"}}", first.ContainerID).Output() //nolint:gosec // test
	if err != nil || strings.TrimSpace(string(labels)) != "true" {
		t.Fatalf("labels %q, %v", labels, err)
	}

	// Applying the same deployment again changes nothing.
	if again := deploy(map[string]string{"A": "1"}); again.ContainerID != first.ContainerID {
		t.Fatalf("redeploy replaced the container: %s -> %s", first.ContainerID, again.ContainerID)
	}
	// A changed configuration recreates the service.
	if changed := deploy(map[string]string{"A": "2"}); changed.ContainerID == first.ContainerID {
		t.Fatal("changed configuration kept the old container")
	}
	if body := httpBody(t, port); body != "deployed" {
		t.Fatalf("service answered %q after the update", body)
	}
}

func TestFailedDeploymentRestoresThePreviousReleaseWithRealDocker(t *testing.T) {
	dockertest.Require(t)
	host := dockertest.StartRegistry(t)
	good := pushImage(t, host, "good", webServer)
	broken := pushImage(t, host, "broken", silentServer)
	d := DeployImageHandler(filepath.Join(t.TempDir(), "deployments"))

	environmentID := uuid.New()
	port, container := freePort(t), 8080
	config := servicecfg.Config{ContainerPort: &container, PublishedPort: &port,
		HealthCheck: servicecfg.HealthCheck{Type: servicecfg.HealthHTTP, Path: "/", TimeoutSeconds: 8, IntervalSeconds: 1}}
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), "docker", "compose", "--project-name", compose.ProjectName(environmentID), "down", "--volumes").Run() //nolint:gosec // test
	})
	deploy := func(image string, previous *agentproto.PreviousRelease) (*logRecorder, error) {
		raw, _ := json.Marshal(agentproto.DeployImagePayload{DeploymentID: uuid.New(), EnvironmentID: environmentID,
			ApplicationID: uuid.New(), CommitSHA: "abc", ImageRef: image, Config: config})
		job := &agentproto.Job{Type: agentproto.JobDeployImage, Payload: raw}
		if previous != nil {
			job.Deploy = &agentproto.DeployContext{Previous: previous}
		}
		log := &logRecorder{}
		_, err := d(context.Background(), job, log)
		return log, err
	}

	if log, err := deploy(good, nil); err != nil {
		t.Fatalf("first deployment: %v\n%s", err, log)
	}
	if body := httpBody(t, port); body != "deployed" {
		t.Fatalf("service answered %q", body)
	}

	log, err := deploy(broken, &agentproto.PreviousRelease{ReleaseID: uuid.New(), CommitSHA: "old", ImageRef: good, Config: config})
	var jobErr *worker.JobError
	if !errors.As(err, &jobErr) || jobErr.Code != "health_check_failed" || !strings.Contains(jobErr.Message, "previous release was restored") {
		t.Fatalf("broken deployment: %v\n%s", err, log)
	}
	if body := httpBody(t, port); body != "deployed" {
		t.Fatalf("after the restore the service answered %q\n%s", body, log)
	}
}
