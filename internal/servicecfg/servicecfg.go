// Package servicecfg defines the service configuration of an environment and
// validates it. Both the Control Plane (on input) and the Agent (before it
// renders a Compose file) use it, so an unsafe option can never be expressed
// and a tampered job payload is rejected at the point of use.
package servicecfg

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/jakkayy/ForgeDeck/internal/validate"
)

// Limits on what a configuration may contain.
const (
	MinPublishedPort    = 1024
	maxEnvVars          = 64
	maxEnvValueBytes    = 4096
	maxVolumes          = 16
	maxSecrets          = 32
	maxPathLength       = 255
	maxHealthPathLength = 255
)

// Binding addresses. Services are not exposed to the network by default.
const (
	BindLoopback = "127.0.0.1"
	BindAll      = "0.0.0.0"
)

// Restart policies.
const (
	RestartUnlessStopped = "unless-stopped"
	RestartAlways        = "always"
	RestartOnFailure     = "on-failure"
	RestartNo            = "no"
)

// Health check types.
const (
	HealthHTTP   = "http"
	HealthTCP    = "tcp"
	HealthDocker = "docker"
)

// SecretName is the form of a project secret's name, which is also the name
// of the environment variable the service receives it in.
var SecretName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)

var (
	envName    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	volumeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	restarts   = map[string]bool{RestartUnlessStopped: true, RestartAlways: true, RestartOnFailure: true, RestartNo: true}
)

// Config describes the one service of an environment.
type Config struct {
	ContainerPort *int              `json:"container_port"`
	PublishedPort *int              `json:"published_port"`
	BindAddress   string            `json:"bind_address"`
	Env           map[string]string `json:"env"`
	// Secrets names project secrets the service receives as environment
	// variables of the same name. The values are never part of a release.
	Secrets     []string    `json:"secrets"`
	Volumes     []Volume    `json:"volumes"`
	Restart     string      `json:"restart"`
	MemoryMB    *int        `json:"memory_mb"`
	CPUs        *float64    `json:"cpus"`
	HealthCheck HealthCheck `json:"health_check"`
}

// Volume mounts a named Docker volume. Host paths are not supported.
type Volume struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// HealthCheck decides when a deployment counts as healthy.
type HealthCheck struct {
	Type            string `json:"type"`
	Path            string `json:"path,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	IntervalSeconds int    `json:"interval_seconds"`
}

// Normalize returns c with defaults applied and fields cleaned, or the
// validation errors keyed by field (nested fields use dots, such as
// "health_check.path"). Only a normalized Config is stored or rendered.
func Normalize(c Config) (Config, error) {
	errs := validate.Errors{}
	c.Env = cloneEnv(c.Env)
	c.Secrets = append([]string{}, c.Secrets...)
	c.Volumes = append([]Volume{}, c.Volumes...)

	checkPorts(errs, c)
	if c.BindAddress == "" {
		c.BindAddress = BindLoopback
	}
	if c.BindAddress != BindLoopback && c.BindAddress != BindAll {
		errs.Add("bind_address", "Use 127.0.0.1 (default) or 0.0.0.0.")
	}
	checkEnv(errs, c.Env)
	checkSecrets(errs, c.Secrets, c.Env)
	checkVolumes(errs, c.Volumes)
	if c.Restart == "" {
		c.Restart = RestartUnlessStopped
	}
	if !restarts[c.Restart] {
		errs.Add("restart", "Use unless-stopped, always, on-failure or no.")
	}
	if c.MemoryMB != nil && (*c.MemoryMB < 16 || *c.MemoryMB > 1<<20) {
		errs.Add("memory_mb", "Memory limit must be 16–1048576 MB.")
	}
	if c.CPUs != nil && (*c.CPUs < 0.01 || *c.CPUs > 256) {
		errs.Add("cpus", "CPU limit must be 0.01–256.")
	}
	c.HealthCheck = normalizeHealthCheck(errs, c.HealthCheck, c.PublishedPort != nil)

	if err := errs.Err(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func checkPorts(errs validate.Errors, c Config) {
	if c.ContainerPort != nil && !validPort(*c.ContainerPort) {
		errs.Add("container_port", "Port must be 1–65535.")
	}
	if c.PublishedPort == nil {
		return
	}
	switch {
	case c.ContainerPort == nil:
		errs.Add("published_port", "Set container_port to publish a port.")
	case *c.PublishedPort < MinPublishedPort || *c.PublishedPort > 65535:
		errs.Add("published_port", fmt.Sprintf("Host port must be %d–65535.", MinPublishedPort))
	}
}

func checkEnv(errs validate.Errors, env map[string]string) {
	if len(env) > maxEnvVars {
		errs.Add("env", fmt.Sprintf("At most %d variables.", maxEnvVars))
	}
	for name, value := range env {
		switch {
		case !envName.MatchString(name):
			errs.Add("env", fmt.Sprintf("%q is not a valid variable name.", name))
		case len(value) > maxEnvValueBytes || strings.ContainsRune(value, 0):
			errs.Add("env", fmt.Sprintf("The value of %s is too long or contains a NUL byte.", name))
		}
	}
}

func checkSecrets(errs validate.Errors, secrets []string, env map[string]string) {
	if len(secrets) > maxSecrets {
		errs.Add("secrets", fmt.Sprintf("At most %d secrets.", maxSecrets))
	}
	seen := map[string]bool{}
	for _, name := range secrets {
		switch {
		case !SecretName.MatchString(name):
			errs.Add("secrets", fmt.Sprintf("%q is not a valid secret name.", name))
		case seen[name]:
			errs.Add("secrets", fmt.Sprintf("%s is listed twice.", name))
		}
		if _, clash := env[name]; clash {
			errs.Add("secrets", fmt.Sprintf("%s is also an environment variable; use one or the other.", name))
		}
		seen[name] = true
	}
}

func checkVolumes(errs validate.Errors, volumes []Volume) {
	if len(volumes) > maxVolumes {
		errs.Add("volumes", fmt.Sprintf("At most %d volumes.", maxVolumes))
	}
	names, paths := map[string]bool{}, map[string]bool{}
	for i := range volumes {
		v := &volumes[i]
		if !volumeName.MatchString(v.Name) {
			errs.Add("volumes", fmt.Sprintf("%q is not a valid volume name. Host paths are not supported.", v.Name))
			continue
		}
		if !validContainerPath(v.Path) {
			errs.Add("volumes", fmt.Sprintf("Mount path %q must be an absolute path inside the container.", v.Path))
			continue
		}
		v.Path = path.Clean(v.Path)
		if names[v.Name] || paths[v.Path] {
			errs.Add("volumes", "Each volume name and mount path may be used once.")
		}
		names[v.Name], paths[v.Path] = true, true
	}
}

func normalizeHealthCheck(errs validate.Errors, h HealthCheck, published bool) HealthCheck {
	if h.Type == "" {
		h.Type = HealthDocker
		if published {
			h.Type = HealthTCP
		}
	}
	if h.TimeoutSeconds == 0 {
		h.TimeoutSeconds = 60
	}
	if h.IntervalSeconds == 0 {
		h.IntervalSeconds = 2
	}
	switch h.Type {
	case HealthHTTP:
		if !published {
			errs.Add("health_check.type", "An http check needs a published_port.")
		}
		if !validHealthPath(h.Path) {
			errs.Add("health_check.path", "Use a path starting with /, such as /healthz.")
		}
	case HealthTCP:
		if !published {
			errs.Add("health_check.type", "A tcp check needs a published_port.")
		}
		h.Path = ""
	case HealthDocker:
		h.Path = ""
	default:
		errs.Add("health_check.type", "Use http, tcp or docker.")
	}
	if h.TimeoutSeconds < 1 || h.TimeoutSeconds > 600 {
		errs.Add("health_check.timeout_seconds", "Timeout must be 1–600 seconds.")
	}
	if h.IntervalSeconds < 1 || h.IntervalSeconds > 60 || h.IntervalSeconds > h.TimeoutSeconds {
		errs.Add("health_check.interval_seconds", "Interval must be 1–60 seconds and not longer than the timeout.")
	}
	return h
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

func validContainerPath(p string) bool {
	return p != "" && len(p) <= maxPathLength && path.IsAbs(p) && path.Clean(p) != "/" &&
		!strings.Contains(p, "..") && !strings.ContainsAny(p, ":\x00\\")
}

// validHealthPath accepts an absolute URL path with an optional query. It
// rejects whitespace and control characters so the path cannot be used to
// smuggle a different request line.
func validHealthPath(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || len(p) > maxHealthPathLength {
		return false
	}
	for _, r := range p {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

func cloneEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}
