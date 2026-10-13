package runner

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

func TestDigestFor(t *testing.T) {
	const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct {
		repoDigests, image, want string
	}{
		{`["ghcr.io/acme/api@` + digest + `"]`, "ghcr.io/acme/api", digest},
		{`["acme/api@` + digest + `"]`, "docker.io/acme/api", digest},
		{`["nginx@` + digest + `"]`, "docker.io/library/nginx", digest},
		{`["localhost:5000/other@` + digest + `","localhost:5000/api@` + digest + `"]`, "localhost:5000/api", digest},
		{`["ghcr.io/acme/other@` + digest + `"]`, "ghcr.io/acme/api", ""},
		{`[]`, "ghcr.io/acme/api", ""},
	}
	for _, tc := range cases {
		got, err := digestFor(tc.repoDigests, tc.image)
		if got != tc.want || (tc.want == "") != (err != nil) {
			t.Errorf("digestFor(%s, %s) = %q, %v", tc.repoDigests, tc.image, got, err)
		}
	}
}

// startRegistry runs a throwaway registry on a free local port and returns
// its host:port. Docker allows plain HTTP to localhost registries.
func startRegistry(t *testing.T) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	name := fmt.Sprintf("forgedeck-test-registry-%d", port)
	out, err := exec.CommandContext(t.Context(), "docker", "run", "--detach", "--rm", "--name", name,
		"--publish", fmt.Sprintf("127.0.0.1:%d:5000", port), "registry:3").CombinedOutput()
	if err != nil {
		t.Fatalf("start registry: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.CommandContext(context.Background(), "docker", "rm", "--force", name).Run() })

	host := fmt.Sprintf("localhost:%d", port)
	deadline := time.Now().Add(15 * time.Second)
	for {
		if conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
			_ = conn.Close()
			return host
		}
		if time.Now().After(deadline) {
			t.Fatal("registry did not start")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestPushImageReturnsDigest(t *testing.T) {
	requireDocker(t)
	host := startRegistry(t)
	ws, _ := newWorkspace(t.TempDir())
	t.Cleanup(func() { _ = ws.remove() })
	log := &recordLog{}
	ex := newExecutor(ws, log)
	if err := ex.run(context.Background(), command{name: "docker", args: []string{"pull", "--quiet", "busybox:1.37"}}); err != nil {
		t.Fatal(err)
	}

	image := host + "/acme/api"
	removeImage(t, image+":abc123")
	registry := agentproto.RegistryCredentials{Host: host, Username: "bot", Password: "registry-password"}
	digest, err := pushImage(context.Background(), ex, registry, "busybox:1.37", image, "abc123")
	if err != nil || !validDigest.MatchString(digest) {
		t.Fatalf("push = %q, %v\n%s", digest, err, log)
	}
	if strings.Contains(log.String(), "registry-password") {
		t.Fatal("registry password logged")
	}
}
