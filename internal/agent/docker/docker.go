// Package docker is the Agent's narrow view of the local Docker Engine
package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

// ErrContainerNotFound means the container no longer exists
var ErrContainerNotFound = errors.New("container not found")

// Only these labels leave the host
const forgedeckLabelPrefix = "forgedeck."

var forwardedComposeLabels = map[string]bool{
	"com.docker.compose.project": true,
	"com.docker.compose.service": true,
}

// Engine wraps a Docker client configured from the standard DOCKER_* env
type Engine struct {
	cli *client.Client
}

// New connects to the Docker Engine (DOCKER_HOST or the local socket)
func New() (*Engine, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Engine{cli: cli}, nil
}

// Close releases the client
func (e *Engine) Close() error { return e.cli.Close() }

// Status reports whether the Engine answers and its version
func (e *Engine) Status(ctx context.Context) agentproto.Docker {
	version, err := e.cli.ServerVersion(ctx, client.ServerVersionOptions{})
	if err != nil {
		return agentproto.Docker{Available: false}
	}
	return agentproto.Docker{Available: true, Version: version.Version}
}

// ListContainers returns every container, running or not
func (e *Engine) ListContainers(ctx context.Context) ([]agentproto.Container, error) {
	result, err := e.cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	containers := make([]agentproto.Container, len(result.Items))
	for i, c := range result.Items {
		containers[i] = agentproto.Container{
			ID:         c.ID,
			Name:       containerName(c.Names),
			Image:      c.Image,
			State:      string(c.State),
			StatusText: c.Status,
			Labels:     forwardedLabels(c.Labels),
		}
	}
	return containers, nil
}

// Start starts a container
func (e *Engine) Start(ctx context.Context, id string) error {
	_, err := e.cli.ContainerStart(ctx, id, client.ContainerStartOptions{})
	return translate(err, "start")
}

// Stop stops a container with the Engine's default grace period
func (e *Engine) Stop(ctx context.Context, id string) error {
	_, err := e.cli.ContainerStop(ctx, id, client.ContainerStopOptions{})
	return translate(err, "stop")
}

// Restart restarts a container with the Engine's default grace period
func (e *Engine) Restart(ctx context.Context, id string) error {
	_, err := e.cli.ContainerRestart(ctx, id, client.ContainerRestartOptions{})
	return translate(err, "restart")
}

// LogOptions selects past output to read
type LogOptions struct {
	TailLines  int
	Since      *time.Time
	Timestamps bool
}

// Logs writes a container's past output to stdout and stderr
func (e *Engine) Logs(ctx context.Context, id string, opts LogOptions, stdout, stderr io.Writer) error {
	inspect, err := e.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return translate(err, "inspect")
	}
	logOpts := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: opts.Timestamps,
		Tail:       strconv.Itoa(opts.TailLines),
	}
	if opts.Since != nil {
		logOpts.Since = strconv.FormatInt(opts.Since.Unix(), 10)
	}
	stream, err := e.cli.ContainerLogs(ctx, id, logOpts)
	if err != nil {
		return translate(err, "read logs")
	}
	defer func() { _ = stream.Close() }()

	if inspect.Container.Config != nil && inspect.Container.Config.Tty {
		_, err = io.Copy(stdout, stream)
	} else {
		_, err = stdcopy.StdCopy(stdout, stderr, stream)
	}
	if err != nil {
		return fmt.Errorf("read logs: %w", err)
	}
	return nil
}

func translate(err error, action string) error {
	switch {
	case err == nil:
		return nil
	case cerrdefs.IsNotFound(err):
		return ErrContainerNotFound
	default:
		return fmt.Errorf("%s container: %w", action, err)
	}
}

// containerName returns Docker's primary name without the leading slash
func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

func forwardedLabels(labels map[string]string) map[string]string {
	forwarded := map[string]string{}
	for key, value := range labels {
		if strings.HasPrefix(key, forgedeckLabelPrefix) || forwardedComposeLabels[key] {
			forwarded[key] = value
		}
	}
	if len(forwarded) == 0 {
		return nil
	}
	return forwarded
}
