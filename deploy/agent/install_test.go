// Package agent_test runs install.sh in a Debian container; opt-in via FORGEDECK_TEST_DOCKER=1.
package agent_test

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

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/dockertest"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

const stubs = `
mkdir -p /stub
cat > /stub/systemctl <<'SH'
#!/bin/sh
echo "systemctl $*" >> /tmp/systemctl.log
exit 0
SH
cat > /stub/docker <<'SH'
#!/bin/sh
[ "$1 $2" = "compose version" ] && echo "Docker Compose version v2.0.0" && exit 0
exit 1
SH
chmod +x /stub/*
groupadd docker
export PATH=/stub:$PATH
`

func run(t *testing.T, env []string, script string, binary string) (string, error) {
	t.Helper()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"run", "--rm", "--network", "host",
		"--volume", filepath.Join(repo, "deploy", "agent") + ":/installer:ro",
		"--volume", binary + ":/agent:ro"}
	for _, e := range env {
		args = append(args, "--env", e)
	}
	args = append(args, "debian:stable-slim", "bash", "-c", stubs+script)
	out, err := exec.CommandContext(t.Context(), "docker", args...).CombinedOutput() //nolint:gosec // test
	return string(out), err
}

func buildAgent(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "forgedeck-agent")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/agent") //nolint:gosec // test
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the Agent: %v\n%s", err, out)
	}
	return binary
}

func TestInstallEnrollsOnceUpgradesAndUninstalls(t *testing.T) {
	dockertest.Require(t)
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	token := owner.IssueEnrollmentToken(projectID, "prod-01")
	binary := buildAgent(t)
	install := "bash /installer/install.sh --binary /agent --url " + env.Server.URL + " --allow-insecure"
	script := `
set -e
` + install + `
echo "--- user:"; id forgedeck-agent
echo "--- binary:"; stat -c '%a %U' /usr/local/bin/forgedeck-agent
echo "--- unit:"; stat -c '%a' /etc/systemd/system/forgedeck-agent.service
echo "--- identity:"; stat -c '%a %U' /var/lib/forgedeck-agent/identity.json; stat -c '%a %U' /var/lib/forgedeck-agent
echo "--- systemctl calls:"; cat /tmp/systemctl.log
echo "--- second run (the token is spent, so enrolling again would fail):"
` + install + `
echo "--- uninstall keeps the identity:"
bash /installer/install.sh --uninstall
ls /var/lib/forgedeck-agent/identity.json; test ! -e /usr/local/bin/forgedeck-agent; test ! -e /etc/systemd/system/forgedeck-agent.service; id forgedeck-agent >/dev/null
echo "--- purge removes it:"
bash /installer/install.sh --uninstall --purge
test ! -e /var/lib/forgedeck-agent; ! id forgedeck-agent 2>/dev/null
echo ALL-OK
`
	out, err := run(t, []string{"FORGEDECK_ENROLLMENT_TOKEN=" + token}, script, binary)
	if err != nil || !strings.Contains(out, "ALL-OK") {
		t.Fatalf("installer: %v\n%s", err, out)
	}
	for _, want := range []string{
		"755 root",            // the binary
		"600 forgedeck-agent", // the identity file holds a credential
		"700 forgedeck-agent", // and its directory
		"systemctl enable forgedeck-agent",
		"systemctl restart forgedeck-agent",
		"already enrolled: keeping the existing identity",
		"groups=",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "docker") {
		t.Errorf("the Agent user is not in the docker group:\n%s", out)
	}

	var list struct{ Items []servers.Server }
	owner.Do(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/servers", nil).RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 1 || list.Items[0].Name != "prod-01" {
		t.Fatalf("servers after two installs: %+v", list.Items)
	}
	if strings.Contains(out, token) {
		t.Errorf("the enrollment token appears in the installer's output")
	}
}

func TestInstallRefusesToEnrollWithoutAToken(t *testing.T) {
	dockertest.Require(t)
	env := apitest.New(t)
	out, err := run(t, nil, "bash /installer/install.sh --binary /agent --url "+env.Server.URL+" --allow-insecure < /dev/null", buildAgent(t))
	if err == nil || !strings.Contains(out, "FORGEDECK_ENROLLMENT_TOKEN") {
		t.Fatalf("a missing token was not reported: %v\n%s", err, out)
	}
}

func TestInstallChecksWhatItNeeds(t *testing.T) {
	dockertest.Require(t)
	binary := buildAgent(t)
	cases := map[string]struct {
		script, want string
	}{
		"no binary and no curl":   {"bash /installer/install.sh --url http://x --allow-insecure", "curl or wget is needed"},
		"binary is missing":       {"bash /installer/install.sh --binary /nowhere --url http://x", "no such file"},
		"not enrolled and no url": {"bash /installer/install.sh --binary /agent", "give --url"},
		"no compose plugin":       {"rm /stub/docker; printf '#!/bin/sh\\nexit 1\\n' > /stub/docker; chmod +x /stub/docker; bash /installer/install.sh --binary /agent --url http://x", "Docker Compose plugin is required"},
		"unknown option":          {"bash /installer/install.sh --nonsense", "unknown option"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := run(t, nil, tc.script, binary)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}

func TestUnitIsValidAndHardened(t *testing.T) {
	data, err := os.ReadFile("forgedeck-agent.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(data)
	for _, want := range []string{
		"User=forgedeck-agent", "SupplementaryGroups=docker", "NoNewPrivileges=true", "ProtectSystem=strict",
		"ProtectHome=true", "PrivateTmp=true", "StateDirectory=forgedeck-agent", "StateDirectoryMode=0700", "Restart=always",
		"ExecStart=/usr/local/bin/forgedeck-agent run --state-dir /var/lib/forgedeck-agent",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("the unit lacks %q", want)
		}
	}
	script, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"binary_dest=/usr/local/bin/forgedeck-agent", "state_dir=/var/lib/forgedeck-agent"} {
		if !strings.Contains(string(script), want) {
			t.Errorf("install.sh does not use %q", want)
		}
	}
	if _, err := exec.LookPath("systemd-analyze"); err == nil {
		// ignore verify noise: the binary is absent here and the host's own units may warn.
		out, _ := exec.CommandContext(t.Context(), "systemd-analyze", "verify", "forgedeck-agent.service").CombinedOutput() //nolint:gosec // test
		for _, line := range strings.Split(string(out), "\n") {
			if line != "" && strings.Contains(line, "forgedeck-agent.service") &&
				!strings.Contains(line, "is not executable") && !strings.Contains(line, "No such file") {
				t.Errorf("systemd-analyze: %s", line)
			}
		}
	}
}

// releaseDir lays out what a GitHub release offers for this CPU and serves it.
func releaseDir(t *testing.T, binary string) (url string, dir string) {
	t.Helper()
	dir = t.TempDir()
	unit, err := os.ReadFile("forgedeck-agent.service")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	name := "forgedeck-agent-linux-" + runtime.GOARCH
	var sums strings.Builder
	for file, content := range map[string][]byte{name: agent, "forgedeck-agent.service": unit} {
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

// curlSetup copies the installer to a lone directory (as a download would) and adds curl to the image.
const curlSetup = `
apt-get update -qq >/dev/null && apt-get install -y -qq curl ca-certificates >/dev/null
mkdir /solo && cp /installer/install.sh /solo/install.sh
`

func TestInstallDownloadsAndVerifiesTheRelease(t *testing.T) {
	dockertest.Require(t)
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	token := owner.IssueEnrollmentToken(projectID, "prod-02")
	binary := buildAgent(t)
	base, _ := releaseDir(t, binary)

	script := curlSetup + `
set -e
bash /solo/install.sh --url ` + env.Server.URL + ` --allow-insecure
test -x /usr/local/bin/forgedeck-agent
test -f /etc/systemd/system/forgedeck-agent.service
echo ALL-OK
`
	out, err := run(t, []string{"FORGEDECK_ENROLLMENT_TOKEN=" + token, "FORGEDECK_RELEASE_BASE=" + base}, script, binary)
	if err != nil || !strings.Contains(out, "ALL-OK") || !strings.Contains(out, "checksum verified") {
		t.Fatalf("installer: %v\n%s", err, out)
	}
	var list struct{ Items []servers.Server }
	owner.Do(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/servers", nil).RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 1 || list.Items[0].Name != "prod-02" {
		t.Fatalf("servers after a downloaded install: %+v", list.Items)
	}
}

func TestInstallRefusesADownloadThatDoesNotMatchItsChecksum(t *testing.T) {
	dockertest.Require(t)
	env := apitest.New(t)
	binary := buildAgent(t)
	base, dir := releaseDir(t, binary)
	if err := os.WriteFile(filepath.Join(dir, "forgedeck-agent-linux-"+runtime.GOARCH), []byte("not the agent"), 0o644); err != nil { //nolint:gosec // test file
		t.Fatal(err)
	}

	script := curlSetup + `
bash /solo/install.sh --url ` + env.Server.URL + ` --allow-insecure
echo "exit=$?"
test ! -e /usr/local/bin/forgedeck-agent && echo NOTHING-INSTALLED
`
	out, _ := run(t, []string{"FORGEDECK_ENROLLMENT_TOKEN=x", "FORGEDECK_RELEASE_BASE=" + base}, script, binary)
	for _, want := range []string{"checksum mismatch", "refusing to install", "NOTHING-INSTALLED"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "exit=0") {
		t.Errorf("the installer succeeded with a corrupted download:\n%s", out)
	}
}
