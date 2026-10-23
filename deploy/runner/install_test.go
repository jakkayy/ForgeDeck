// Package runner_test runs install.sh in a Debian container; opt-in via FORGEDECK_TEST_DOCKER=1.
package runner_test

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/dockertest"
)

const stubs = `
mkdir -p /stub
printf '#!/bin/sh\necho "systemctl $*" >> /tmp/systemctl.log\nexit 0\n' > /stub/systemctl
chmod +x /stub/systemctl
export PATH=/stub:$PATH
`

// curlSetup copies the installer to a lone directory (as a download would) and adds curl to the image.
const curlSetup = `
apt-get update -qq >/dev/null && apt-get install -y -qq curl ca-certificates >/dev/null
mkdir /solo && cp /installer/install.sh /solo/install.sh
`

const credential = "fdr_test-credential-0123456789"

func run(t *testing.T, env []string, script, binary string) (string, error) {
	t.Helper()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"run", "--rm", "--network", "host",
		"--volume", filepath.Join(repo, "deploy", "runner") + ":/installer:ro",
		"--volume", binary + ":/runner:ro"}
	for _, e := range env {
		args = append(args, "--env", e)
	}
	args = append(args, "debian:stable-slim", "bash", "-c", stubs+script)
	out, err := exec.CommandContext(t.Context(), "docker", args...).CombinedOutput() //nolint:gosec // test
	return string(out), err
}

func buildRunner(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "forgedeck-runner")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/runner") //nolint:gosec // test
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the Runner: %v\n%s", err, out)
	}
	return binary
}

func TestInstallWritesTheConfigurationAndKeepsItOnUpgrade(t *testing.T) {
	dockertest.Require(t)
	binary := buildRunner(t)
	script := `
set -e
bash /installer/install.sh --binary /runner --url https://forgedeck.example.com
echo "--- env file:"; stat -c '%a %U:%G' /etc/forgedeck-runner/runner.env
grep -c '^FORGEDECK_URL=https://forgedeck.example.com$' /etc/forgedeck-runner/runner.env
grep -c '^FORGEDECK_RUNNER_CREDENTIAL=` + credential + `$' /etc/forgedeck-runner/runner.env
echo "--- binary and unit:"; stat -c '%a' /usr/local/bin/forgedeck-runner /etc/systemd/system/forgedeck-runner.service
echo "--- second run keeps the file (a different credential must not replace it):"
FORGEDECK_RUNNER_CREDENTIAL=fdr_other bash /installer/install.sh --binary /runner
grep -c '^FORGEDECK_RUNNER_CREDENTIAL=` + credential + `$' /etc/forgedeck-runner/runner.env
echo "--- uninstall keeps the configuration:"
bash /installer/install.sh --uninstall
test ! -e /usr/local/bin/forgedeck-runner; test ! -e /etc/systemd/system/forgedeck-runner.service; test -f /etc/forgedeck-runner/runner.env
echo ALL-OK
`
	out, err := run(t, []string{"FORGEDECK_RUNNER_CREDENTIAL=" + credential}, script, binary)
	if err != nil || !strings.Contains(out, "ALL-OK") {
		t.Fatalf("installer: %v\n%s", err, out)
	}
	for _, want := range []string{"640 root:forgedeck-runner", "already configured: keeping"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, credential) {
		t.Errorf("the credential appears in the installer's output")
	}
}

func TestInstallChecksWhatItNeeds(t *testing.T) {
	dockertest.Require(t)
	binary := buildRunner(t)
	cases := map[string]struct {
		env          []string
		script, want string
	}{
		"not configured and no url":    {nil, "bash /installer/install.sh --binary /runner", "give --url"},
		"credential of the wrong kind": {[]string{"FORGEDECK_RUNNER_CREDENTIAL=fde_enrollment-token"}, "bash /installer/install.sh --binary /runner --url https://x", "does not look like a Runner credential"},
		"binary is missing":            {nil, "bash /installer/install.sh --binary /nowhere --url https://x", "no such file"},
		"no credential and no tty":     {nil, "bash /installer/install.sh --binary /runner --url https://x < /dev/null", "FORGEDECK_RUNNER_CREDENTIAL"},
		"no binary and no curl":        {nil, "bash /installer/install.sh --url https://x", "curl or wget is needed"},
		"unknown option":               {nil, "bash /installer/install.sh --nonsense", "unknown option"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := run(t, tc.env, tc.script, binary)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}

// releaseDir lays out what a GitHub release offers for this CPU and serves it.
func releaseDir(t *testing.T, binary string) (url, dir string) {
	t.Helper()
	dir = t.TempDir()
	unit, err := os.ReadFile("forgedeck-runner.service")
	if err != nil {
		t.Fatal(err)
	}
	runner, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	name := "forgedeck-runner-linux-" + runtime.GOARCH
	var sums strings.Builder
	for file, content := range map[string][]byte{name: runner, "forgedeck-runner.service": unit} {
		if err := os.WriteFile(filepath.Join(dir, file), content, 0o644); err != nil { //nolint:gosec // test files
			t.Fatal(err)
		}
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(content), file)
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sums.String()), 0o644); err != nil { //nolint:gosec // test file
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(server.Close)
	return server.URL, dir
}

func TestInstallDownloadsAndVerifiesTheRelease(t *testing.T) {
	dockertest.Require(t)
	binary := buildRunner(t)
	base, _ := releaseDir(t, binary)
	script := curlSetup + `
set -e
bash /solo/install.sh --url https://forgedeck.example.com
test -x /usr/local/bin/forgedeck-runner
test -f /etc/systemd/system/forgedeck-runner.service
test -f /etc/forgedeck-runner/runner.env
echo ALL-OK
`
	out, err := run(t, []string{"FORGEDECK_RUNNER_CREDENTIAL=" + credential, "FORGEDECK_RELEASE_BASE=" + base}, script, binary)
	if err != nil || !strings.Contains(out, "ALL-OK") || !strings.Contains(out, "checksum verified") {
		t.Fatalf("installer: %v\n%s", err, out)
	}
}

func TestInstallRefusesADownloadThatDoesNotMatchItsChecksum(t *testing.T) {
	dockertest.Require(t)
	binary := buildRunner(t)
	base, dir := releaseDir(t, binary)
	if err := os.WriteFile(filepath.Join(dir, "forgedeck-runner-linux-"+runtime.GOARCH), []byte("not the runner"), 0o644); err != nil { //nolint:gosec // test file
		t.Fatal(err)
	}
	script := curlSetup + `
bash /solo/install.sh --url https://forgedeck.example.com
echo "exit=$?"
test ! -e /usr/local/bin/forgedeck-runner && echo NOTHING-INSTALLED
`
	out, _ := run(t, []string{"FORGEDECK_RUNNER_CREDENTIAL=" + credential, "FORGEDECK_RELEASE_BASE=" + base}, script, binary)
	for _, want := range []string{"checksum mismatch", "refusing to install", "NOTHING-INSTALLED"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "exit=0") {
		t.Errorf("the installer succeeded with a corrupted download:\n%s", out)
	}
}
