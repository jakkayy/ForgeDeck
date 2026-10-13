package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/procexec"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
)

const (
	// healthyProbes is how many probes in a row must pass. One pass can be
	// luck: a crash-looping container may answer once between restarts.
	healthyProbes = 2
	probeTimeout  = 5 * time.Second
)

// probeFuncs are the checks that talk to the service from the host.
type probeFuncs struct {
	http func(ctx context.Context, port int, path string) error
	tcp  func(ctx context.Context, port int) error
}

func hostProbes() probeFuncs {
	// A redirect is a response from a live service, so it counts as healthy.
	client := &http.Client{Timeout: probeTimeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return probeFuncs{
		http: func(ctx context.Context, port int, path string) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+path, nil)
			if err != nil {
				return err
			}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 400 {
				return fmt.Errorf("GET %s returned %d", path, resp.StatusCode)
			}
			return nil
		},
		tcp: func(ctx context.Context, port int) error {
			ctx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", "127.0.0.1:"+strconv.Itoa(port))
			if err != nil {
				return err
			}
			return conn.Close()
		},
	}
}

// waitHealthy polls the service until it has passed healthyProbes probes in a
// row, or fails when the container stops running or the timeout passes. A
// running container is not enough: the configured check must pass.
func (d *deployer) waitHealthy(ctx context.Context, runner commandRunner, containerID string, config servicecfg.Config) error {
	check := config.HealthCheck
	deadline := d.now().Add(time.Duration(check.TimeoutSeconds) * time.Second)
	interval := time.Duration(check.IntervalSeconds) * time.Second
	passed := 0
	var last error
	for {
		state, err := inspectContainer(ctx, runner, containerID)
		if err != nil {
			return err
		}
		if state.Status != "running" || state.Restarting {
			return fmt.Errorf("the container stopped (status %s, exit code %d)", state.Status, state.ExitCode)
		}

		if last = d.probe(ctx, check, config, state); last == nil {
			if passed++; passed >= healthyProbes {
				return nil
			}
		} else {
			passed = 0
		}
		if !d.now().Add(interval).Before(deadline) {
			return fmt.Errorf("not healthy after %d seconds: %w", check.TimeoutSeconds, last)
		}
		if err := d.sleep(ctx, interval); err != nil {
			return err
		}
	}
}

func (d *deployer) probe(ctx context.Context, check servicecfg.HealthCheck, config servicecfg.Config, state containerState) error {
	if check.Type != servicecfg.HealthDocker && config.PublishedPort == nil {
		return errors.New("the health check needs a published port")
	}
	switch check.Type {
	case servicecfg.HealthHTTP:
		return d.probes.http(ctx, *config.PublishedPort, check.Path)
	case servicecfg.HealthTCP:
		return d.probes.tcp(ctx, *config.PublishedPort)
	case servicecfg.HealthDocker:
		switch state.Health {
		case "healthy":
			return nil
		case "":
			return errors.New("the image defines no HEALTHCHECK; choose an http or tcp check instead")
		default:
			return fmt.Errorf("the image's health check reports %s", state.Health)
		}
	default:
		return fmt.Errorf("unknown health check type %q", check.Type)
	}
}

// containerState is what a health check needs from `docker inspect`.
type containerState struct {
	Status     string
	Restarting bool
	ExitCode   int
	Health     string // "", "starting", "healthy", "unhealthy"
}

const inspectFormat = `{{.State.Status}} {{.State.Restarting}} {{.State.ExitCode}} {{if .State.Health}}{{.State.Health.Status}}{{end}}`

func inspectContainer(ctx context.Context, runner commandRunner, containerID string) (containerState, error) {
	out, err := runner.Output(ctx, procexec.Command{Name: "docker", Args: []string{"inspect", "--format", inspectFormat, containerID}})
	if err != nil {
		return containerState{}, fmt.Errorf("inspect container: %w", err)
	}
	fields := strings.Fields(out)
	if len(fields) < 3 {
		return containerState{}, fmt.Errorf("unexpected inspect output %q", out)
	}
	exit, _ := strconv.Atoi(fields[2])
	state := containerState{Status: fields[0], Restarting: fields[1] == "true", ExitCode: exit}
	if len(fields) > 3 {
		state.Health = fields[3]
	}
	return state, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
