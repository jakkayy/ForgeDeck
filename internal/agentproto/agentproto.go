// Package agentproto defines the wire contract between the Control Plane and
// the ForgeDeck Agent. Both sides import it, so changes here are protocol
// changes: bump ProtocolVersion for incompatible ones.
package agentproto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
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

// MaxSnapshotContainers caps one snapshot; the Control Plane rejects more.
const MaxSnapshotContainers = 500

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
	// Credentials are minted for this attempt of a build job. They are sent
	// only in the claim response, never stored or logged.
	Credentials *BuildCredentials `json:"credentials,omitempty"`
}

// BuildCredentials let a Runner fetch one repository and push its image.
type BuildCredentials struct {
	Git      GitCredentials      `json:"git"`
	Registry RegistryCredentials `json:"registry"`
	// Image is the repository the build is pushed to, e.g. ghcr.io/acme/shop-api.
	Image string `json:"image"`
}

// RegistryCredentials authenticate docker login to one registry.
type RegistryCredentials struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// GitCredentials authenticate an HTTPS clone with HTTP Basic auth.
type GitCredentials struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
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

// Outcomes an Agent may report for a job attempt.
const (
	OutcomeSucceeded = "succeeded"
	OutcomeFailed    = "failed"
)

// MaxResultBytes bounds the structured result stored with a job.
const MaxResultBytes = 64 * 1024

// CompleteRequest reports the final outcome of an attempt. Resending the
// same outcome is safe. ErrorCode is a stable machine-readable code such as
// "container_not_found"; ErrorMessage is shown to users.
type CompleteRequest struct {
	Attempt      int32           `json:"attempt"`
	Outcome      string          `json:"outcome"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorCode    string          `json:"error_code,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
}

// Job types. Agents run container and deployment jobs; Runners run builds.
const (
	JobContainerAction = "container_action"
	JobFetchLogs       = "fetch_logs"
	JobDeployImage     = "deploy_image"
	JobBuild           = "build"
)

// BuildResult is a successful build's job result. Deployments use
// ImageRef, which pins the exact image pushed.
type BuildResult struct {
	CommitSHA string `json:"commit_sha"`
	Image     string `json:"image"`     // e.g. ghcr.io/acme/shop-api
	Tag       string `json:"tag"`       // short commit SHA
	Digest    string `json:"digest"`    // sha256:…
	ImageRef  string `json:"image_ref"` // Image@Digest
}

// DeployImagePayload deploys one image digest to one environment on the
// Agent's server. It never carries secrets: registry credentials are added
// to the claimed job, not stored in the payload. The Agent re-validates
// Config before rendering it.
type DeployImagePayload struct {
	DeploymentID  uuid.UUID         `json:"deployment_id"`
	EnvironmentID uuid.UUID         `json:"environment_id"`
	ApplicationID uuid.UUID         `json:"application_id"`
	CommitSHA     string            `json:"commit_sha"`
	ImageRef      string            `json:"image_ref"` // registry/repo@sha256:…
	Config        servicecfg.Config `json:"config"`
}

// BuildPayload builds and pushes an image for one commit. The Runner checks
// out exactly CommitSHA, never the moving branch.
type BuildPayload struct {
	ApplicationID      uuid.UUID `json:"application_id"`
	GitConnectionID    uuid.UUID `json:"git_connection_id"`
	RepositoryID       string    `json:"repository_id"`
	RepositoryFullName string    `json:"repository_full_name"`
	Branch             string    `json:"branch"`
	CommitSHA          string    `json:"commit_sha"`
	CommitMessage      string    `json:"commit_message,omitempty"`
	DockerfilePath     string    `json:"dockerfile_path"`
	BuildContext       string    `json:"build_context"`
	TestCommand        *string   `json:"test_command,omitempty"`
	// Trigger says why the build exists: TriggerPush, TriggerManual (branch
	// head), or TriggerCommit (an explicit commit, which is not auto-deployed).
	Trigger string `json:"trigger,omitempty"`
}

// Build triggers.
const (
	TriggerPush   = "push"
	TriggerManual = "manual"
	TriggerCommit = "commit"
)

// Container actions accepted by JobContainerAction.
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
)

// ContainerActionPayload starts, stops, or restarts one container. Applying
// an action that is already in effect (start a running container) succeeds.
type ContainerActionPayload struct {
	ContainerID string `json:"container_id"`
	Action      string `json:"action"`
}

// Log fetch limits. The Agent enforces them too, even if a payload asks for more.
const (
	DefaultTailLines = 500
	MaxTailLines     = 2000
	MaxFetchLogBytes = 1 << 20
)

// FetchLogsPayload reads a bounded range of a container's past output. The
// Agent streams it back as job events and reports truncation in the result.
type FetchLogsPayload struct {
	ContainerID string     `json:"container_id"`
	TailLines   int        `json:"tail_lines"`
	Since       *time.Time `json:"since,omitempty"`
	Timestamps  bool       `json:"timestamps"`
}
