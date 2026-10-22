package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/procexec"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

const testImage = "ghcr.io/acme/shop-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type fakeDocker struct {
	mu         sync.Mutex
	commands   []procexec.Command
	failOn     string // a word that must appear in the command's arguments
	loginStdin string
	env        []string
	// inspectOut is what `docker inspect` prints; empty means running.
	inspectOut string
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
	joined := strings.Join(c.Args, " ")
	switch {
	case strings.HasPrefix(joined, "inspect"):
		return f.inspect(), nil
	case strings.HasSuffix(joined, "ps --quiet app"):
		return "abc123def456\nsecondline", nil
	default:
		return "v5.6.0", nil
	}
}

func (f *fakeDocker) inspect() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inspectOut != "" {
		return f.inspectOut
	}
	return "running false 0 "
}

func (f *fakeDocker) count(word string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.commands {
		if strings.Contains(strings.Join(c.Args, " "), word) {
			n++
		}
	}
	return n
}

// composeBaseArgs is how many arguments precede the compose subcommand.
const composeBaseArgs = 7

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

// fakeClock makes sleeping instant but advances time, so health-check timeouts run fast.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) sleep(_ context.Context, d time.Duration) error {
	c.t = c.t.Add(d)
	return nil
}

type passingProbes struct {
	mu     sync.Mutex
	http   []string
	tcp    int
	failFn func() error // when set, decides each probe's result
}

func (p *passingProbes) funcs() probeFuncs {
	result := func() error {
		if p.failFn != nil {
			return p.failFn()
		}
		return nil
	}
	return probeFuncs{
		http: func(_ context.Context, port int, path string) error {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.http = append(p.http, fmt.Sprintf("%d%s", port, path))
			return result()
		},
		tcp: func(context.Context, int) error {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.tcp++
			return result()
		},
	}
}

func newTestDeployer(t *testing.T, fake *fakeDocker) *deployer {
	t.Helper()
	return newTestDeployerWith(t, fake, &passingProbes{})
}

func newTestDeployerWith(t *testing.T, fake *fakeDocker, probes *passingProbes) *deployer {
	t.Helper()
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	return &deployer{dir: filepath.Join(t.TempDir(), "deployments"),
		newRunner: func(env []string, _ worker.Logger) commandRunner {
			fake.env = env
			return fake
		},
		probes: probes.funcs(), sleep: clock.sleep, now: clock.now}
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
	want := []string{"compose version", "login", "compose pull", "compose up --detach --remove-orphans --no-build --pull never", "compose ps --quiet app", "inspect", "inspect"}
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

	composeFile := filepath.Join(d.dir, payload.EnvironmentID.String(), "compose.yml")
	data, err := os.ReadFile(composeFile)
	if err != nil || !strings.Contains(string(data), testImage) {
		t.Fatalf("compose file: %v\n%s", err, data)
	}
	if info, _ := os.Stat(composeFile); info.Mode().Perm() != 0o600 {
		t.Fatalf("compose file mode %v", info.Mode().Perm())
	}
	// registry logins use the job's own Docker config, not the Agent's.
	if cfg := envValue(fake.env, "DOCKER_CONFIG"); !strings.HasPrefix(cfg, d.dir+string(filepath.Separator)+"job-") {
		t.Fatalf("DOCKER_CONFIG %q is not job-private", cfg)
	}
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

func healthJob(t *testing.T, check servicecfg.HealthCheck, previous *agentproto.PreviousRelease) *agentproto.Job {
	t.Helper()
	job, payload := deployJob(t, nil)
	payload.Config.HealthCheck = check
	raw, _ := json.Marshal(payload)
	job.Payload = raw
	if previous != nil {
		job.Deploy = &agentproto.DeployContext{Previous: previous}
	}
	return job
}

func previousRelease() *agentproto.PreviousRelease {
	port, container := 18080, 8080
	return &agentproto.PreviousRelease{ReleaseID: uuid.New(), CommitSHA: "old",
		ImageRef: "ghcr.io/acme/shop-api@sha256:" + strings.Repeat("a", 64),
		Config:   servicecfg.Config{ContainerPort: &container, PublishedPort: &port}}
}

func TestHealthCheckTypes(t *testing.T) {
	probes := &passingProbes{}
	fake := &fakeDocker{}
	job := healthJob(t, servicecfg.HealthCheck{Type: servicecfg.HealthHTTP, Path: "/healthz"}, nil)
	if _, err := newTestDeployerWith(t, fake, probes).handle(context.Background(), job, &logRecorder{}); err != nil {
		t.Fatal(err)
	}
	if len(probes.http) != healthyProbes || probes.http[0] != "18080/healthz" {
		t.Fatalf("http probes %v", probes.http)
	}

	fake = &fakeDocker{inspectOut: "running false 0 healthy"}
	job = healthJob(t, servicecfg.HealthCheck{Type: servicecfg.HealthDocker}, nil)
	if _, err := newTestDeployer(t, fake).handle(context.Background(), job, &logRecorder{}); err != nil {
		t.Fatalf("docker health check: %v", err)
	}
}

func TestUnhealthyServiceWithoutPreviousReleaseIsStopped(t *testing.T) {
	probes := &passingProbes{failFn: func() error { return errors.New("connection refused") }}
	fake := &fakeDocker{}
	log := &logRecorder{}
	job := healthJob(t, servicecfg.HealthCheck{Type: servicecfg.HealthTCP, TimeoutSeconds: 10, IntervalSeconds: 2}, nil)

	_, err := newTestDeployerWith(t, fake, probes).handle(context.Background(), job, log)
	var jobErr *worker.JobError
	if !errors.As(err, &jobErr) || jobErr.Code != "health_check_failed" || !strings.Contains(jobErr.Message, "no previous release") {
		t.Fatalf("got %v", err)
	}
	if fake.count("logs --tail 50") != 1 {
		t.Fatalf("the failed service's output was not kept: %v", fake.subcommands())
	}
	if fake.count(" stop") != 1 {
		t.Fatalf("the failed service was not stopped: %v", fake.subcommands())
	}
	if !strings.Contains(log.String(), "connection refused") || !strings.Contains(log.String(), "not healthy after 10 seconds") {
		t.Fatalf("log:\n%s", log)
	}
}

func TestUnhealthyServiceRestoresThePreviousRelease(t *testing.T) {
	previous := previousRelease()
	fake := &fakeDocker{}
	probes := &passingProbes{}
	probes.failFn = func() error {
		if fake.count("up --detach") >= 2 {
			return nil
		}
		return errors.New("connection refused")
	}
	log := &logRecorder{}
	job := healthJob(t, servicecfg.HealthCheck{Type: servicecfg.HealthTCP, TimeoutSeconds: 10, IntervalSeconds: 2}, previous)
	d := newTestDeployerWith(t, fake, probes)

	_, err := d.handle(context.Background(), job, log)
	var jobErr *worker.JobError
	if !errors.As(err, &jobErr) || jobErr.Code != "health_check_failed" || !strings.Contains(jobErr.Message, "previous release was restored") {
		t.Fatalf("got %v\n%s", err, log)
	}
	if fake.count("up --detach") != 2 || fake.count(" stop") != 0 {
		t.Fatalf("commands %v", fake.subcommands())
	}
	var payload agentproto.DeployImagePayload
	_ = json.Unmarshal(job.Payload, &payload)
	data, readErr := os.ReadFile(filepath.Join(d.dir, payload.EnvironmentID.String(), "compose.yml"))
	if readErr != nil || !strings.Contains(string(data), previous.ImageRef) {
		t.Fatalf("compose file after restore: %v\n%s", readErr, data)
	}
	// restoring tolerates a local image even if the pull fails.
	if fake.count("pull --ignore-pull-failures") != 1 {
		t.Fatalf("commands %v", fake.subcommands())
	}
}

func TestFailedRestoreIsReported(t *testing.T) {
	fake := &fakeDocker{}
	probes := &passingProbes{failFn: func() error { return errors.New("down") }}
	job := healthJob(t, servicecfg.HealthCheck{Type: servicecfg.HealthTCP, TimeoutSeconds: 4, IntervalSeconds: 2}, previousRelease())

	_, err := newTestDeployerWith(t, fake, probes).handle(context.Background(), job, &logRecorder{})
	var jobErr *worker.JobError
	if !errors.As(err, &jobErr) || !strings.Contains(jobErr.Message, "restoring the previous release failed") {
		t.Fatalf("got %v", err)
	}
}

func TestContainerThatExitsFailsImmediately(t *testing.T) {
	fake := &fakeDocker{inspectOut: "exited false 137 "}
	probes := &passingProbes{}
	log := &logRecorder{}
	job := healthJob(t, servicecfg.HealthCheck{Type: servicecfg.HealthTCP, TimeoutSeconds: 600, IntervalSeconds: 2}, nil)

	if _, err := newTestDeployerWith(t, fake, probes).handle(context.Background(), job, log); err == nil {
		t.Fatal("deploy succeeded")
	}
	if probes.tcp != 0 || !strings.Contains(log.String(), "exit code 137") {
		t.Fatalf("probed %d times\n%s", probes.tcp, log)
	}
}

func TestDockerHealthCheckNeedsAnImageHealthcheck(t *testing.T) {
	fake := &fakeDocker{}
	job := healthJob(t, servicecfg.HealthCheck{Type: servicecfg.HealthDocker, TimeoutSeconds: 4, IntervalSeconds: 2}, nil)
	log := &logRecorder{}
	if _, err := newTestDeployer(t, fake).handle(context.Background(), job, log); err == nil {
		t.Fatal("deploy succeeded")
	}
	if !strings.Contains(log.String(), "defines no HEALTHCHECK") {
		t.Fatalf("log:\n%s", log)
	}
}

func TestOneLuckyProbeIsNotEnough(t *testing.T) {
	calls := 0
	probes := &passingProbes{failFn: func() error {
		calls++
		if calls == 2 { // pass, fail, pass, pass
			return errors.New("flapping")
		}
		return nil
	}}
	job := healthJob(t, servicecfg.HealthCheck{Type: servicecfg.HealthTCP, TimeoutSeconds: 60, IntervalSeconds: 2}, nil)
	if _, err := newTestDeployerWith(t, &fakeDocker{}, probes).handle(context.Background(), job, &logRecorder{}); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatalf("probed %d times, want 4 (pass, fail, pass, pass)", calls)
	}
}

func TestSecretsAreMaskedInTheJobLog(t *testing.T) {
	out := &logRecorder{}
	port := 18080
	config := servicecfg.Config{PublishedPort: &port, Env: map[string]string{"DB_PASSWORD": "hunter2-secret"}}
	log := redactSecrets(out, &agentproto.DeployContext{Registry: &agentproto.RegistryCredentials{Password: "registry-secret"}}, config)
	log.Log("stdout", "connecting with hunter2-secret via registry-secret")
	if got := out.String(); strings.Contains(got, "hunter2") || strings.Contains(got, "registry-secret") {
		t.Fatalf("leaked: %s", got)
	}
}

func secretJob(t *testing.T, secrets []string, values map[string]string, previous *agentproto.PreviousRelease) *agentproto.Job {
	t.Helper()
	job, payload := deployJob(t, nil)
	payload.Config.Secrets = secrets
	raw, _ := json.Marshal(payload)
	job.Payload = raw
	job.Deploy = &agentproto.DeployContext{Secrets: values, Previous: previous}
	return job
}

func TestSecretsReachComposeThroughTheEnvironmentOnly(t *testing.T) {
	fake := &fakeDocker{}
	d := newTestDeployer(t, fake)
	job := secretJob(t, []string{"DATABASE_PASSWORD"},
		map[string]string{"DATABASE_PASSWORD": "db-secret-value", "UNRELATED": "other-project-secret"}, nil)
	var payload agentproto.DeployImagePayload
	_ = json.Unmarshal(job.Payload, &payload)

	if _, err := d.handle(context.Background(), job, &logRecorder{}); err != nil {
		t.Fatal(err)
	}
	for _, c := range fake.commands {
		if len(c.Args) > 0 && c.Args[0] == "compose" && len(c.Args) > composeBaseArgs {
			if strings.Join(c.Env, " ") != "DATABASE_PASSWORD=db-secret-value" {
				t.Fatalf("compose %v ran with environment %v", c.Args[composeBaseArgs:], c.Env)
			}
		}
		if strings.Contains(strings.Join(c.Args, " "), "db-secret-value") {
			t.Fatalf("secret in arguments: %v", c.Args)
		}
	}
	data, err := os.ReadFile(filepath.Join(d.dir, payload.EnvironmentID.String(), "compose.yml"))
	if err != nil || !strings.Contains(string(data), "${DATABASE_PASSWORD}") || strings.Contains(string(data), "db-secret-value") {
		t.Fatalf("compose file: %v\n%s", err, data)
	}
}

func TestDeploymentWithoutADeliveredSecretFailsBeforeRunningAnything(t *testing.T) {
	fake := &fakeDocker{}
	job := secretJob(t, []string{"DATABASE_PASSWORD"}, map[string]string{}, nil)
	_, err := newTestDeployer(t, fake).handle(context.Background(), job, &logRecorder{})
	var jobErr *worker.JobError
	if !errors.As(err, &jobErr) || jobErr.Code != "secret_not_found" || !strings.Contains(jobErr.Message, "DATABASE_PASSWORD") {
		t.Fatalf("got %v", err)
	}
	if len(fake.commands) != 0 {
		t.Fatalf("ran %d commands", len(fake.commands))
	}
}

func TestPreviousReleaseWithADeletedSecretIsNotRestored(t *testing.T) {
	previous := previousRelease()
	previous.Config.Secrets = []string{"GONE_SECRET"}
	fake := &fakeDocker{}
	probes := &passingProbes{failFn: func() error { return errors.New("down") }}
	job := secretJob(t, nil, nil, previous)
	var payload agentproto.DeployImagePayload
	_ = json.Unmarshal(job.Payload, &payload)
	payload.Config.HealthCheck = servicecfg.HealthCheck{Type: servicecfg.HealthTCP, TimeoutSeconds: 4, IntervalSeconds: 2}
	job.Payload, _ = json.Marshal(payload)
	log := &logRecorder{}

	_, err := newTestDeployerWith(t, fake, probes).handle(context.Background(), job, log)
	var jobErr *worker.JobError
	if !errors.As(err, &jobErr) || !strings.Contains(jobErr.Message, "no previous release") {
		t.Fatalf("got %v", err)
	}
	if fake.count("up --detach") != 1 || !strings.Contains(log.String(), "GONE_SECRET") {
		t.Fatalf("commands %v\n%s", fake.subcommands(), log)
	}
}

func TestSecretValuesAreMaskedInTheJobLogOfADeployment(t *testing.T) {
	out := &logRecorder{}
	log := redactSecrets(out, &agentproto.DeployContext{Secrets: map[string]string{"API_KEY": "key-0123456789"}}, servicecfg.Config{})
	log.Log("stdout", "calling with key-0123456789")
	if strings.Contains(out.String(), "key-0123456789") {
		t.Fatalf("leaked: %s", out)
	}
}
