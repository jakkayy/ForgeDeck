package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveInsideRefusesEscapes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "services", "api"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "Evil.Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../..", filepath.Join(root, "services", "up")); err != nil {
		t.Fatal(err)
	}

	for _, ok := range []string{"Dockerfile", ".", "services/api"} {
		if _, err := resolveInside(root, ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"Evil.Dockerfile", "services/up", "../outside"} {
		if _, err := resolveInside(root, bad); err == nil {
			t.Errorf("%s resolved", bad)
		}
	}
	if _, err := resolveInside(root, "Evil.Dockerfile"); !errors.Is(err, errOutsideCheckout) {
		t.Errorf("symlink escape: %v", err)
	}
}

// requireDocker skips tests that need a Docker daemon unless opted in.
func requireDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("FORGEDECK_TEST_DOCKER") != "1" {
		t.Skip("set FORGEDECK_TEST_DOCKER=1 to run against a Docker daemon")
	}
}

// removeImage deletes a test image, ignoring errors.
func removeImage(t *testing.T, ref string) {
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), "docker", "image", "rm", "--force", ref).Run()
	})
}

func TestBuildImageWithBuildKit(t *testing.T) {
	requireDocker(t)
	url, commit := sourceRepo(t, map[string]string{
		"app/Dockerfile": "FROM busybox:1.37\nCOPY hello.txt /hello.txt\nRUN cat /hello.txt\n",
		"app/hello.txt":  "hello from forgedeck\n",
	})
	ws, _ := newWorkspace(t.TempDir())
	t.Cleanup(func() { _ = ws.remove() })
	log := &recordLog{}
	ex := newExecutor(ws, log)
	if err := checkout(context.Background(), ex, ws.src, gitCredentials(url), commit, "file"); err != nil {
		t.Fatal(err)
	}

	tag := "forgedeck-build:test-" + commit[:12]
	removeImage(t, tag)
	if err := buildImage(context.Background(), ex, ws.src, "app/Dockerfile", "app", commit, tag); err != nil {
		t.Fatalf("build: %v\n%s", err, log)
	}
	out, err := exec.CommandContext(context.Background(), "docker", "image", "inspect", "--format",
		`{{index .Config.Labels "org.opencontainers.image.revision"}}`, tag).Output()
	if err != nil || string(out) != commit+"\n" {
		t.Fatalf("revision label %q, %v", out, err)
	}
}
