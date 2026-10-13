package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jakkayy/ForgeDeck/internal/agent/compose"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/procexec"
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
}

// DeployImageHandler applies a deploy_image job: it renders the Compose file
// from the validated configuration, pulls the image by digest, and brings
// the service up. Applying is idempotent: running the same deployment again
// converges to the same state. dir is the Agent's deployment directory.
func DeployImageHandler(dir string) worker.Handler {
	d := &deployer{dir: dir, newRunner: func(env []string, log worker.Logger) commandRunner {
		return procexec.New(env, log)
	}}
	return d.handle
}

func (d *deployer) handle(ctx context.Context, job *agentproto.Job, log worker.Logger) (any, error) {
	var payload agentproto.DeployImagePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, errInvalidPayload
	}
	file, err := compose.Render(compose.Spec{
		EnvironmentID: payload.EnvironmentID, ImageRef: payload.ImageRef, Config: payload.Config,
	})
	if err != nil {
		return nil, &worker.JobError{Code: "invalid_payload", Message: err.Error()}
	}

	scratch, err := d.newScratch()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch.root) }()
	if job.Deploy != nil && job.Deploy.Registry != nil {
		log = worker.NewRedactingLog(log, []string{job.Deploy.Registry.Password})
	}
	runner := d.newRunner(procexec.BaseEnv(scratch.home, scratch.dockerConfig), log)

	envDir, err := d.writeComposeFile(payload.EnvironmentID.String(), file)
	if err != nil {
		return nil, err
	}
	project := composeCommand{runner: runner, project: compose.ProjectName(payload.EnvironmentID), dir: envDir}

	if _, err := runner.Output(ctx, procexec.Command{Name: "docker", Args: []string{"compose", "version"}}); err != nil {
		return nil, errComposeUnavailable
	}
	if job.Deploy != nil && job.Deploy.Registry != nil {
		log.Log(agentproto.StreamSystem, "Logging in to "+job.Deploy.Registry.Host)
		if err := login(ctx, runner, *job.Deploy.Registry); err != nil {
			log.Log(agentproto.StreamSystem, err.Error())
			return nil, errRegistryLogin
		}
	}

	log.Log(agentproto.StreamSystem, "Pulling "+payload.ImageRef)
	if err := project.run(ctx, "pull"); err != nil {
		log.Log(agentproto.StreamSystem, err.Error())
		return nil, errPullFailed
	}
	log.Log(agentproto.StreamSystem, "Starting the service")
	if err := project.run(ctx, "up", "--detach", "--remove-orphans", "--no-build", "--pull", "never"); err != nil {
		log.Log(agentproto.StreamSystem, err.Error())
		return nil, errStartFailed
	}

	containerID, err := project.output(ctx, "ps", "--quiet", compose.ServiceName)
	if err != nil || containerID == "" {
		return nil, errStartFailed
	}
	return agentproto.DeployResult{ImageRef: payload.ImageRef, ContainerID: firstLine(containerID)}, nil
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
