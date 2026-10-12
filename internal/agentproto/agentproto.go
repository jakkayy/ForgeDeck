// Package agentproto defines the wire contract between the Control Plane and
// the ForgeDeck Agent. Both sides import it, so changes here are protocol
// changes: bump ProtocolVersion for incompatible ones.
package agentproto

import (
	"time"

	"github.com/google/uuid"
)

// ProtocolVersion is the Agent protocol spoken by this build.
const ProtocolVersion = 1

// CredentialPrefix marks Agent credentials so they are recognizable in
// accidental logs and secret scanners.
const CredentialPrefix = "fda_"

// EnrollRequest exchanges a single-use enrollment token for a credential.
type EnrollRequest struct {
	Token           string `json:"token"`
	AgentVersion    string `json:"agent_version"`
	ProtocolVersion int    `json:"protocol_version"`
}

// EnrollResponse carries the Agent's long-lived credential. It is returned
// once; the Control Plane keeps only its hash.
type EnrollResponse struct {
	AgentID    uuid.UUID `json:"agent_id"`
	ServerID   uuid.UUID `json:"server_id"`
	Credential string    `json:"credential"`
}

// HeartbeatRequest reports liveness and a status summary.
type HeartbeatRequest struct {
	AgentVersion    string     `json:"agent_version"`
	ProtocolVersion int        `json:"protocol_version"`
	Docker          Docker     `json:"docker"`
	Resources       *Resources `json:"resources,omitempty"`
}

// Docker describes the local Docker Engine as seen by the Agent.
type Docker struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
}

// Resources is a host usage summary in percent (0–100).
type Resources struct {
	CPUPercent    float32 `json:"cpu_percent"`
	MemoryPercent float32 `json:"memory_percent"`
	DiskPercent   float32 `json:"disk_percent"`
}

// HeartbeatResponse tells the Agent when to send the next heartbeat.
type HeartbeatResponse struct {
	IntervalSeconds int `json:"interval_seconds"`
}

// ContainerSnapshot is the full list of containers on the Agent's host at
// ObservedAt. It replaces the previous snapshot; it is not a diff.
type ContainerSnapshot struct {
	ObservedAt time.Time   `json:"observed_at"`
	Containers []Container `json:"containers"`
}

// Container is one Docker container. It deliberately omits environment
// variables, mounts, and other inspect data that may contain secrets.
type Container struct {
	ID         string            `json:"container_id"`
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	State      string            `json:"state"`
	StatusText string            `json:"status_text,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// SnapshotResponse reports whether the snapshot replaced the stored one.
// A snapshot older than the stored one is ignored, not an error.
type SnapshotResponse struct {
	Accepted bool `json:"accepted"`
}
