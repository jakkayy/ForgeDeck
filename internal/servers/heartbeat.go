package servers

import (
	"context"
	"fmt"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

// HeartbeatInterval is how often Agents are asked to report in.
const HeartbeatInterval = 20 * time.Second

const maxVersionLength = 64

// RecordHeartbeat stores the Agent's status; last_seen_at uses the Control Plane clock
func (s *Service) RecordHeartbeat(ctx context.Context, identity AgentIdentity, hb agentproto.HeartbeatRequest) error {
	if err := validateHeartbeat(hb); err != nil {
		return err
	}
	params := store.RecordHeartbeatParams{
		ID:              identity.ServerID,
		Now:             s.now(),
		AgentVersion:    &hb.AgentVersion,
		DockerAvailable: hb.Docker.Available,
		DockerVersion:   optional(hb.Docker.Version),
	}
	if r := hb.Resources; r != nil {
		params.CpuPercent, params.MemoryPercent, params.DiskPercent = &r.CPUPercent, &r.MemoryPercent, &r.DiskPercent
	}
	if err := s.queries.RecordHeartbeat(ctx, params); err != nil {
		return fmt.Errorf("record heartbeat: %w", err)
	}
	return nil
}

func validateHeartbeat(hb agentproto.HeartbeatRequest) error {
	errs := validate.Errors{}
	if len(hb.AgentVersion) > maxVersionLength {
		errs.Add("agent_version", "Agent version is too long.")
	}
	if len(hb.Docker.Version) > maxVersionLength {
		errs.Add("docker.version", "Docker version is too long.")
	}
	if r := hb.Resources; r != nil {
		for field, v := range map[string]float32{
			"resources.cpu_percent": r.CPUPercent, "resources.memory_percent": r.MemoryPercent, "resources.disk_percent": r.DiskPercent,
		} {
			if v < 0 || v > 100 {
				errs.Add(field, "Must be between 0 and 100.")
			}
		}
	}
	return errs.Err()
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
