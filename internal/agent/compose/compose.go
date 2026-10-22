// Package compose renders the Compose file of an environment's service
package compose

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
)

// ServiceName is the name of the single service of every environment
const ServiceName = "app"

// Labels the Agent puts on the service's containers
const (
	LabelManaged     = "forgedeck.managed"
	LabelEnvironment = "forgedeck.environment"
)

var imageRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]*@sha256:[0-9a-f]{64}$`)

// Spec is one service to run
type Spec struct {
	EnvironmentID uuid.UUID
	ImageRef      string // registry/repo@sha256:…
	Config        servicecfg.Config
}

// ProjectName is the Compose project of an environment
func ProjectName(environmentID uuid.UUID) string {
	return "forgedeck-" + environmentID.String()
}

// Render returns the Compose file for spec
func Render(spec Spec) ([]byte, error) {
	config, err := servicecfg.Normalize(spec.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid service configuration: %w", err)
	}
	if !imageRef.MatchString(spec.ImageRef) {
		return nil, fmt.Errorf("image %q is not pinned to a digest", spec.ImageRef)
	}

	service := map[string]any{
		"image":   spec.ImageRef,
		"restart": config.Restart,
		"labels": map[string]string{
			LabelManaged:     "true",
			LabelEnvironment: spec.EnvironmentID.String(),
		},
	}
	if config.ContainerPort != nil && config.PublishedPort != nil {
		service["ports"] = []string{fmt.Sprintf("%s:%d:%d", config.BindAddress, *config.PublishedPort, *config.ContainerPort)}
	}
	if len(config.Env) > 0 || len(config.Secrets) > 0 {
		env := make(map[string]string, len(config.Env)+len(config.Secrets))
		for name, value := range config.Env {
			env[name] = escapeInterpolation(value)
		}
		// A secret's value is not in the file
		for _, name := range config.Secrets {
			env[name] = "${" + name + "}"
		}
		service["environment"] = env
	}
	volumes := map[string]any{}
	if len(config.Volumes) > 0 {
		mounts := make([]string, len(config.Volumes))
		for i, v := range config.Volumes {
			mounts[i] = v.Name + ":" + v.Path
			volumes[v.Name] = map[string]any{}
		}
		service["volumes"] = mounts
	}
	if config.MemoryMB != nil {
		service["mem_limit"] = fmt.Sprintf("%dm", *config.MemoryMB)
	}
	if config.CPUs != nil {
		service["cpus"] = *config.CPUs
	}

	file := map[string]any{
		"name":     ProjectName(spec.EnvironmentID),
		"services": map[string]any{ServiceName: service},
	}
	if len(volumes) > 0 {
		file["volumes"] = maps.Clone(volumes)
	}
	return json.MarshalIndent(file, "", "  ")
}

// escapeInterpolation protects a value from Compose's own variable interpolation
func escapeInterpolation(value string) string {
	return strings.ReplaceAll(value, "$", "$$")
}
