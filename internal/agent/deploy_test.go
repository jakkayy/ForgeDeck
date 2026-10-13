package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/procexec"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

const testImage = "ghcr.io/acme/shop-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// fakeDocker records commands and fails the ones a test names.
type fakeDocker struct {
	mu       sync.Mutex
	commands []procexec.Command
	failOn   string // a word that must appear in the command's arguments
	// dockerConfigs holds what DOCKER_CONFIG contained when login ran.
	loginStdin string
	env        []string
}

func (f *fakeDocker) record(c procexec.Command) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, c)
	if f.failOn != "" && strings.Contains(strings.Join(c.Args, " "), f.failOn) {
		return errors.New("exit status 1")
	}
	return nil
}

func (f *fakeDocker) Run(_ context.Context, c procexec.Command) error {
	if c.Stdin != nil {
		data := new(strings.Builder)
		buf := make([]byte, 256)
		for {
			n, err := c.Stdin.Read(buf)
			data.Write(buf[:n])
			if err != nil {
				break
			}
		}
		f.loginStdin = data.String()
	}
	return f.record(c)
}

func (f *fakeDocker) Output(_ context.Context, c procexec.Command) (string, error) {
	if err := f.record(c); err != nil {
		return "", err
	}
	if len(c.Args) > 0 && c.Args[len(c.Args)-2] == "--quiet" {
		return "abc123def456\nsecondline", nil
	}
	return "v5.6.0", nil
}

// composeBaseArgs is how many arguments precede the subcommand:
// compose --project-name P --file F --project-directory D.
const composeBaseArgs = 7

// subcommands summarizes the recorded commands, such as "compose pull".
func (f *fakeDocker) subcommands() []string {
	var out []string
	for _, c := range f.commands {
		switch {
		case len(c.Args) > 1 && c.Args[0] == "compose" && c.Args[1] == "version":
			out = append(out, "compose version")
		case len(c.Args) > composeBaseArgs && c.Args[0] == "compose":
			out = append(out, "compose "+strings.Join(c.Args[composeBaseArgs:], " "))
		default:
			out = append(out, c.Args[0])
		}
	}
	return out
}

type logRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (l *logRecorder) Log(stream, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, stream+": "+message)
}

func (l *logRecorder) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func newTestDeployer(t *testing.T, fake *fakeDocker) *deployer {
	t.Helper()
	return &deployer{dir: filepath.Join(t.TempDir(), "deployments"),
		newRunner: func(env []string, _ worker.Logger) commandRunner {
			fake.env = env
			return fake
		}}
}

func deployJob(t *testing.T, registry *agentproto.RegistryCredentials) (*agentproto.Job, agentproto.DeployImagePayload) {
	t.Helper()
	port, container := 18080, 8080
	payload := agentproto.DeployImagePayload{
		DeploymentID: uuid.New(), EnvironmentID: uuid.New(), ApplicationID: uuid.New(), CommitSHA: "abc", ImageRef: testImage,
		Config: servicecfg.Config{ContainerPort: &container, PublishedPort: &port, Env: map[string]string{"A": "1"}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	job := &agentproto.Job{ID: uuid.New(), Type: agentproto.JobDeployImage, Payload: raw}
	if registry != nil {
		job.Deploy = &agentproto.DeployContext{Registry: registry}
	}
	return job, payload
}

func TestDeployPullsThenStartsAndReportsTheContainer(t *testing.T) {
	fake := &fakeDocker{}
	d := newTestDeployer(t, fake)
	job, payload := deployJob(t, &agentproto.RegistryCredentials{Host: "ghcr.io", Username: "bot", Password: "registry-secret"})
	log := &logRecorder{}

	result, err := d.handle(context.Background(), job, log)
	if err != nil {
		t.Fatalf("deploy: %v\n%s", err, log)
	}
	got, ok := result.(agentproto.DeployResult)
	if !ok || got.ImageRef != testImage || got.ContainerID != "abc123def456" {
		t.Fatalf("result %+v", result)
	}
	want := []string{"compose version", "login", "compose pull", "compose up --detach --remove-orphans --no-build --pull never", "compose ps --quiet app"}
	if cmds := fake.subcommands(); strings.Join(cmds, "|") != strings.Join(want, "|") {
		t.Fatalf("commands %v, want %v", cmds, want)
	}
	if fake.loginStdin != "registry-secret" {
		t.Fatalf("login stdin %q", fake.loginStdin)
	}
	for _, c := range fake.commands {
		if strings.Contains(strings.Join(c.Args, " "), "registry-secret") {
			t.Fatalf("password in arguments: %v", c.Args)
		}
	}

	// The compose file is in place for the environment and names the digest.
	composeFile := filepath.Join(d.dir, payload.EnvironmentID.String(), "compose.yml")
	data, err := os.ReadFile(composeFile)
	if err != nil || !strings.Contains(string(data), testImage) {
		t.Fatalf("compose file: %v\n%s", err, data)
	}
	if info, _ := os.Stat(composeFile); info.Mode().Perm() != 0o600 {
		t.Fatalf("compose file mode %v", info.Mode().Perm())
	}
	// Registry logins happen in the job's own Docker config, not the Agent's.
	if cfg := envValue(fake.env, "DOCKER_CONFIG"); !strings.HasPrefix(cfg, d.dir+string(filepath.Separator)+"job-") {
		t.Fatalf("DOCKER_CONFIG %q is not job-private", cfg)
	}
	// Nothing of the job outlives it: no scratch directory, no registry login.
	entries, _ := os.ReadDir(d.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "job-") {
			t.Fatalf("scratch directory left behind: %s", e.Name())
		}
	}
}

func TestDeployWithoutRegistryDoesNotLogin(t *testing.T) {
	fake := &fakeDocker{}
	job, _ := deployJob(t, nil)
	if _, err := newTestDeployer(t, fake).handle(context.Background(), job, &logRecorder{}); err != nil {
		t.Fatal(err)
	}
	for _, sub := range fake.subcommands() {
		if sub == "login" {
			t.Fatal("logged in without credentials")
		}
	}
}

func TestDeployMapsFailuresToStepCodes(t *testing.T) {
	cases := map[string]struct{ failOn, code string }{
		"compose missing": {"version", "compose_unavailable"},
		"login rejected":  {"login", "registry_login_failed"},
		"pull fails":      {" pull", "pull_failed"},
		"up fails":        {" up ", "start_failed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := &fakeDocker{failOn: tc.failOn}
			job, _ := deployJob(t, &agentproto.RegistryCredentials{Host: "ghcr.io", Username: "bot", Password: "secret"})
			_, err := newTestDeployer(t, fake).handle(context.Background(), job, &logRecorder{})
			var jobErr *worker.JobError
			if !errors.As(err, &jobErr) || jobErr.Code != tc.code {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
		})
	}
}

func TestDeployRefusesInvalidPayloads(t *testing.T) {
	job, payload := deployJob(t, nil)
	cases := map[string]func(*agentproto.DeployImagePayload){
		"mutable tag": func(p *agentproto.DeployImagePayload) { p.ImageRef = "ghcr.io/acme/app:latest" },
		"host mount": func(p *agentproto.DeployImagePayload) {
			p.Config.Volumes = []servicecfg.Volume{{Name: "/etc", Path: "/x"}}
		},
		"low host port":   func(p *agentproto.DeployImagePayload) { p.Config.PublishedPort = ptrInt(22) },
		"unknown restart": func(p *agentproto.DeployImagePayload) { p.Config.Restart = "sometimes" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := payload
			mutate(&p)
			raw, _ := json.Marshal(p)
			fake := &fakeDocker{}
			_, err := newTestDeployer(t, fake).handle(context.Background(), &agentproto.Job{Payload: raw, Type: job.Type}, &logRecorder{})
			var jobErr *worker.JobError
			if !errors.As(err, &jobErr) || jobErr.Code != "invalid_payload" {
				t.Fatalf("got %v", err)
			}
			if len(fake.commands) != 0 {
				t.Fatalf("ran %d commands for an invalid payload", len(fake.commands))
			}
		})
	}
	if _, err := newTestDeployer(t, &fakeDocker{}).handle(context.Background(), &agentproto.Job{Payload: []byte("{")}, &logRecorder{}); err == nil {
		t.Fatal("garbage payload accepted")
	}
}

func ptrInt(v int) *int { return &v }

func envValue(env []string, key string) string {
	for _, kv := range env {
		if value, ok := strings.CutPrefix(kv, key+"="); ok {
			return value
		}
	}
	return ""
}
