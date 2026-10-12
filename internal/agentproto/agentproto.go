// Package agentproto defines the wire contract between the Control Plane and
// the ForgeDeck Agent. Both sides import it, so changes here are protocol
// changes: bump ProtocolVersion for incompatible ones.
package agentproto

import "github.com/google/uuid"

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
