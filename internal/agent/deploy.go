package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agent/compose"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/procexec"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// Deployment failures. The code tells the Control Plane (and the user) which
// step failed.
var (
	errComposeUnavailable = &worker.JobError{Code: "compose_unavailable", Message: "Docker Compose is not available on this server. Install the Docker Compose plugin."}
	errRegistryLogin      = &worker.JobError{Code: "registry_login_failed", Message: "The registry rejected the credentials. Check the registry in the project."}
	errPullFailed         = &worker.JobError{Code: "pull_failed", Message: "The image could not be pulled. See the job log."}
	errStartFailed        = &worker.JobError{Code: "start_failed", Message: "The service could not be started. See the job log."}
)

// commandRunner runs the Docker CLI. procexec.Executor implements it.
type commandRunner interface {
	Run(ctx context.Context, c procexec.Command) error
	Output(ctx context.Context, c procexec.Command) (string, error)
}

// deployer applies deployments on this server with Docker Compose.
type deployer struct {
	// dir holds one subdirectory per environment with its compose.yml, and
	// short-lived scratch directories for jobs.
	dir string
	// newRunner builds the command runner of one job. env is its base
	// environment; log receives the command output.
	newRunner func(env []string, log worker.Logger) commandRunner
	// probes check a service from the host; tests replace them.
	probes probeFuncs
	// sleep waits between health probes; tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time
}

// DeployImageHandler applies a deploy_image job: it renders the Compose file
// from the validated configuration, pulls the image by digest, and brings
// the service up. Applying is idempotent: running the same deployment again
// converges to the same state. dir is the Agent's deployment directory.
func DeployImageHandler(dir string) worker.Handler {
	d := &deployer{dir: dir, newRunner: func(env []string, log worker.Logger) commandRunner {
		return procexec.New(env, log)
	}, probes: hostProbes(), sleep: sleepContext, now: time.Now}
	return d.handle
}

func (d *deployer) handle(ctx context.Context, job *agentproto.Job, log worker.Logger) (any, error) {
	var payload agentproto.DeployImagePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, errInvalidPayload
	}
	// The configuration is validated here, on the server, and its defaults
	// are filled in once, so every step below uses the same values. An
	// invalid payload fails before anything runs.
	config, err := servicecfg.Normalize(payload.Config)
	if err != nil {
		return nil, &worker.JobError{Code: "invalid_payload", Message: err.Error()}
	}
	target := release{ImageRef: payload.ImageRef, Config: config}
	if _, err := target.render(payload.EnvironmentID); err != nil {
		return nil, &worker.JobError{Code: "invalid_payload", Message: err.Error()}
	}

	scratch, err := d.newScratch()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch.root) }()
	log = redactSecrets(log, job.Deploy, config)
	a := &attempt{
		d: d, runner: d.newRunner(procexec.BaseEnv(scratch.home, scratch.dockerConfig), log), log: log,
		environmentID: payload.EnvironmentID, registry: registryOf(job.Deploy),
	}

	if _, err := a.runner.Output(ctx, procexec.Command{Name: "docker", Args: []string{"compose", "version"}}); err != nil {
		return nil, errComposeUnavailable
	}
	if a.registry != nil {
		log.Log(agentproto.StreamSystem, agentproto.StageLine(agentproto.StagePull, "Logging in to "+a.registry.Host))
		if err := login(ctx, a.runner, *a.registry); err != nil {
			log.Log(agentproto.StreamSystem, err.Error())
			return nil, errRegistryLogin
		}
	}

	containerID, err := a.apply(ctx, target, true)
	if err != nil {
		return nil, err
	}
	log.Log(agentproto.StreamSystem, agentproto.StageLine(agentproto.StageHealthCheck, "Waiting for the service to become healthy"))
	if err := d.waitHealthy(ctx, a.runner, containerID, config); err != nil {
		log.Log(agentproto.StreamSystem, "Health check failed: "+err.Error())
		return nil, a.recover(ctx, containerID, previousOf(job.Deploy, log))
	}
	log.Log(agentproto.StreamSystem, "The service is healthy")
	return agentproto.DeployResult{ImageRef: payload.ImageRef, ContainerID: containerID}, nil
}

// release is an image and the configuration to run it with.
type release struct {
	ImageRef string
	Config   servicecfg.Config
}

func (r release) render(environmentID uuid.UUID) ([]byte, error) {
	return compose.Render(compose.Spec{EnvironmentID: environmentID, ImageRef: r.ImageRef, Config: r.Config})
}

// attempt is one job's deployment of an environment.
type attempt struct {
	d             *deployer
	runner        commandRunner
	log           worker.Logger
	environmentID uuid.UUID
	registry      *agentproto.RegistryCredentials
}

// apply writes the Compose file of r and brings the service up, returning its
// container ID. With mustPull the image is pulled first and a failed pull
// fails the step; otherwise a failed pull is tolerated because the image may
// already be on the host (restoring a previous release).
func (r *attempt) apply(ctx context.Context, rel release, mustPull bool) (string, error) {
	file, err := rel.render(r.environmentID)
	if err != nil {
		return "", &worker.JobError{Code: "invalid_payload", Message: err.Error()}
	}
	envDir, err := r.d.writeComposeFile(r.environmentID.String(), file)
	if err != nil {
		return "", err
	}
	project := composeCommand{runner: r.runner, project: compose.ProjectName(r.environmentID), dir: envDir}

	r.log.Log(agentproto.StreamSystem, agentproto.StageLine(agentproto.StagePull, "Pulling "+rel.ImageRef))
	pullArgs := []string{"pull"}
	if !mustPull {
		pullArgs = append(pullArgs, "--ignore-pull-failures")
	}
	if err := project.run(ctx, pullArgs...); err != nil {
		r.log.Log(agentproto.StreamSystem, err.Error())
		return "", errPullFailed
	}
	r.log.Log(agentproto.StreamSystem, agentproto.StageLine(agentproto.StageStart, "Starting the service"))
	if err := project.run(ctx, "up", "--detach", "--remove-orphans", "--no-build", "--pull", "never"); err != nil {
		r.log.Log(agentproto.StreamSystem, err.Error())
		return "", errStartFailed
	}
	containerID, err := project.output(ctx, "ps", "--quiet", compose.ServiceName)
	if err != nil || containerID == "" {
		return "", errStartFailed
	}
	return firstLine(containerID), nil
}

// recover handles a service that did not become healthy: it keeps the
// container's last output for diagnosis, then restores the previous release.
// With no previous release the failed service is stopped so it does not
// crash-loop. The returned error is always the failed health check; its
// message says what state the environment is in.
func (r *attempt) recover(ctx context.Context, failedContainerID string, previous *release) error {
	r.keepLogs(ctx, failedContainerID)
	if previous == nil {
		r.log.Log(agentproto.StreamSystem, "No previous release to restore; stopping the failed service")
		if err := r.stop(ctx); err != nil {
			r.log.Log(agentproto.StreamSystem, "Stopping the service failed: "+err.Error())
		}
		return &worker.JobError{Code: "health_check_failed", Message: "The service did not become healthy and was stopped. There was no previous release to restore."}
	}

	r.log.Log(agentproto.StreamSystem, "Restoring the previous release "+previous.ImageRef)
	containerID, err := r.apply(ctx, *previous, false)
	if err == nil {
		err = r.d.waitHealthy(ctx, r.runner, containerID, previous.Config)
	}
	if err != nil {
		r.log.Log(agentproto.StreamSystem, "Restoring the previous release failed: "+err.Error())
		return &worker.JobError{Code: "health_check_failed", Message: "The service did not become healthy, and restoring the previous release failed too. The service may be down."}
	}
	r.log.Log(agentproto.StreamSystem, "The previous release is running again")
	return &worker.JobError{Code: "health_check_failed", Message: "The service did not become healthy. The previous release was restored and is running."}
}

func (r *attempt) stop(ctx context.Context) error {
	dir := filepath.Join(r.d.dir, r.environmentID.String())
	return composeCommand{runner: r.runner, project: compose.ProjectName(r.environmentID), dir: dir}.run(ctx, "stop")
}

// keepLogs copies the last lines of a container's output into the job log.
func (r *attempt) keepLogs(ctx context.Context, containerID string) {
	r.log.Log(agentproto.StreamSystem, "Last output of the failed service:")
	if err := r.runner.Run(ctx, procexec.Command{Name: "docker", Args: []string{"logs", "--tail", "50", containerID}}); err != nil {
		r.log.Log(agentproto.StreamSystem, "Could not read the container's output: "+err.Error())
	}
}

func registryOf(c *agentproto.DeployContext) *agentproto.RegistryCredentials {
	if c == nil {
		return nil
	}
	return c.Registry
}

// previousOf returns the release to restore, validated like the new one. A
// previous release that no longer validates is not restored: running an
// unchecked configuration is worse than leaving the service stopped.
func previousOf(c *agentproto.DeployContext, log worker.Logger) *release {
	if c == nil || c.Previous == nil {
		return nil
	}
	config, err := servicecfg.Normalize(c.Previous.Config)
	if err != nil {
		log.Log(agentproto.StreamSystem, "The previous release's configuration is not valid and will not be restored: "+err.Error())
		return nil
	}
	return &release{ImageRef: c.Previous.ImageRef, Config: config}
}

// redactSecrets masks the registry password and the environment values of
// the configuration in everything the job logs, including a failed
// service's own output.
func redactSecrets(log worker.Logger, c *agentproto.DeployContext, config servicecfg.Config) worker.Logger {
	var secrets []string
	if c != nil && c.Registry != nil {
		secrets = append(secrets, c.Registry.Password)
	}
	for _, value := range config.Env {
		secrets = append(secrets, value)
	}
	if c != nil && c.Previous != nil {
		for _, value := range c.Previous.Config.Env {
			secrets = append(secrets, value)
		}
	}
	return worker.NewRedactingLog(log, secrets)
}

// scratch is a job's private directory tree. HOME and DOCKER_CONFIG point
// into it so a registry login never touches the Agent's own files and
// disappears with the job.
type scratch struct{ root, home, dockerConfig string }

func (d *deployer) newScratch() (scratch, error) {
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return scratch{}, fmt.Errorf("create deployment directory: %w", err)
	}
	root, err := os.MkdirTemp(d.dir, "job-")
	if err != nil {
		return scratch{}, fmt.Errorf("create job directory: %w", err)
	}
	s := scratch{root: root, home: filepath.Join(root, "home"), dockerConfig: filepath.Join(root, "docker")}
	for _, dir := range []string{s.home, s.dockerConfig} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			_ = os.RemoveAll(root)
			return scratch{}, fmt.Errorf("create job directory: %w", err)
		}
	}
	return s, nil
}

// writeComposeFile stores the environment's Compose file, replacing the
// previous one atomically, and returns the environment's directory.
func (d *deployer) writeComposeFile(environmentID string, content []byte) (string, error) {
	envDir := filepath.Join(d.dir, environmentID)
	if err := os.MkdirAll(envDir, 0o700); err != nil {
		return "", fmt.Errorf("create environment directory: %w", err)
	}
	tmp, err := os.CreateTemp(envDir, "compose-*.tmp")
	if err != nil {
		return "", fmt.Errorf("write compose file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write compose file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("write compose file: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(envDir, composeFileName)); err != nil {
		return "", fmt.Errorf("write compose file: %w", err)
	}
	return envDir, nil
}

const composeFileName = "compose.yml"

func login(ctx context.Context, runner commandRunner, registry agentproto.RegistryCredentials) error {
	return runner.Run(ctx, procexec.Command{
		Name:  "docker",
		Args:  []string{"login", "--username", registry.Username, "--password-stdin", registry.Host},
		Stdin: strings.NewReader(registry.Password),
	})
}

// composeCommand runs `docker compose` for one environment's project.
type composeCommand struct {
	runner  commandRunner
	project string
	dir     string
}

func (c composeCommand) args(sub ...string) []string {
	base := []string{"compose", "--project-name", c.project,
		"--file", filepath.Join(c.dir, composeFileName), "--project-directory", c.dir}
	return append(base, sub...)
}

func (c composeCommand) run(ctx context.Context, sub ...string) error {
	return c.runner.Run(ctx, procexec.Command{Name: "docker", Args: c.args(sub...), Dir: c.dir})
}

func (c composeCommand) output(ctx context.Context, sub ...string) (string, error) {
	return c.runner.Output(ctx, procexec.Command{Name: "docker", Args: c.args(sub...), Dir: c.dir})
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
