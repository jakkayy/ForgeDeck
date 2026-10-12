// Package agentproto defines the wire contract between the Control Plane and
// the ForgeDeck Agent. Both sides import it, so changes here are protocol
// changes: bump ProtocolVersion for incompatible ones.
package agentproto

import (
	"encoding/json"
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

// MaxClaimWaitSeconds caps how long a claim request is held open. It stays
// below common reverse-proxy idle timeouts (often 30–60 seconds).
const MaxClaimWaitSeconds = 25

// ClaimRequest asks for the next job targeted at this Agent's server.
// WaitSeconds (0–MaxClaimWaitSeconds) is how long to hold the request open
// when nothing is queued; omitted means the maximum.
type ClaimRequest struct {
	WaitSeconds *int `json:"wait_seconds,omitempty"`
}

// Job is a job assigned to the Agent. The response is 204 No Content when
// no job became available within the wait.
type Job struct {
	ID             uuid.UUID       `json:"id"`
	Type           string          `json:"type"`
	Attempt        int32           `json:"attempt"`
	Payload        json.RawMessage `json:"payload"`
	LeaseExpiresAt time.Time       `json:"lease_expires_at"`
}

// LeaseRequest renews the lease on a claimed job attempt.
type LeaseRequest struct {
	Attempt int32 `json:"attempt"`
}

// LeaseResponse carries the new lease expiry. A 409 with code "lease_lost"
// means the attempt no longer belongs to this Agent and must be abandoned.
type LeaseResponse struct {
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}

// Log streams accepted in job events.
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
	StreamSystem = "system"
)

// Batch limits for job events.
const (
	MaxEventsPerBatch = 500
	MaxEventBytes     = 16 * 1024
)

// EventsRequest appends log lines for a job attempt. Sequence numbers
// increase monotonically per attempt; resending a batch is safe because
// duplicates are ignored.
type EventsRequest struct {
	Attempt int32      `json:"attempt"`
	Events  []LogEvent `json:"events"`
}

// LogEvent is one line of job output. Secrets must be redacted by the Agent
// before sending.
type LogEvent struct {
	Sequence int64  `json:"sequence"`
	Stream   string `json:"stream"`
	Message  string `json:"message"`
}
