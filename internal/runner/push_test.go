package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/dockertest"
	"github.com/jakkayy/ForgeDeck/internal/procexec"
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

func TestPushImageReturnsDigest(t *testing.T) {
	dockertest.Require(t)
	host := dockertest.StartRegistry(t)
	ws, _ := newWorkspace(t.TempDir())
	t.Cleanup(func() { _ = ws.remove() })
	log := &recordLog{}
	ex := newExecutor(ws, log)
	if err := ex.Run(context.Background(), procexec.Command{Name: "docker", Args: []string{"pull", "--quiet", "busybox:1.37"}}); err != nil {
		t.Fatal(err)
	}

	image := host + "/acme/api"
	dockertest.RemoveImage(t, image+":abc123")
	registry := agentproto.RegistryCredentials{Host: host, Username: "bot", Password: "registry-password"}
	digest, err := pushImage(context.Background(), ex, registry, "busybox:1.37", image, "abc123")
	if err != nil || !validDigest.MatchString(digest) {
		t.Fatalf("push = %q, %v\n%s", digest, err, log)
	}
	if strings.Contains(log.String(), "registry-password") {
		t.Fatal("registry password logged")
	}
}
