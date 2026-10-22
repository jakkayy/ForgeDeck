// Package agentproto defines the wire contract between the Control Plane and the ForgeDeck Agent
package agentproto

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
)

// ProtocolVersion is the Agent protocol spoken by this build
const ProtocolVersion = 1

// CredentialPrefix makes Agent credentials recognizable to secret scanners
const CredentialPrefix = "fda_"

// EnrollRequest exchanges a single-use enrollment token for a credential
type EnrollRequest struct {
	Token           string `json:"token"`
	AgentVersion    string `json:"agent_version"`
	ProtocolVersion int    `json:"protocol_version"`
}

// EnrollResponse carries the Agent's long-lived credential
type EnrollResponse struct {
	AgentID    uuid.UUID `json:"agent_id"`
	ServerID   uuid.UUID `json:"server_id"`
	Credential string    `json:"credential"`
}

// HeartbeatRequest reports liveness and a status summary
type HeartbeatRequest struct {
	AgentVersion    string     `json:"agent_version"`
	ProtocolVersion int        `json:"protocol_version"`
	Docker          Docker     `json:"docker"`
	Resources       *Resources `json:"resources,omitempty"`
}

// Docker describes the local Docker Engine as seen by the Agent
type Docker struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
}

// Resources is a host usage summary in percent (0–100)
type Resources struct {
	CPUPercent    float32 `json:"cpu_percent"`
	MemoryPercent float32 `json:"memory_percent"`
	DiskPercent   float32 `json:"disk_percent"`
}

// HeartbeatResponse tells the Agent when to send the next heartbeat
type HeartbeatResponse struct {
	IntervalSeconds  int  `json:"interval_seconds"`
	RotateCredential bool `json:"rotate_credential,omitempty"`
}

// RotateResponse carries the Agent's new credential
type RotateResponse struct {
	ServerID   uuid.UUID `json:"server_id"`
	Credential string    `json:"credential"`
}

// MaxSnapshotContainers caps one snapshot
const MaxSnapshotContainers = 500

// ContainerSnapshot is the full list of containers on the Agent's host at ObservedAt
type ContainerSnapshot struct {
	ObservedAt time.Time   `json:"observed_at"`
	Containers []Container `json:"containers"`
}

// Container is one Docker container
type Container struct {
	ID         string            `json:"container_id"`
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	State      string            `json:"state"`
	StatusText string            `json:"status_text,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// SnapshotResponse reports whether the snapshot replaced the stored one
type SnapshotResponse struct {
	Accepted bool `json:"accepted"`
}

// MaxClaimWaitSeconds caps how long a claim request is held open
const MaxClaimWaitSeconds = 25

// ClaimRequest asks for the next job targeted at this Agent's server
type ClaimRequest struct {
	WaitSeconds *int `json:"wait_seconds,omitempty"`
}

// Job is a job assigned to the Agent
type Job struct {
	ID             uuid.UUID       `json:"id"`
	Type           string          `json:"type"`
	Attempt        int32           `json:"attempt"`
	Payload        json.RawMessage `json:"payload"`
	LeaseExpiresAt time.Time       `json:"lease_expires_at"`
	// they are sent only in the claim response, never stored or logged
	Credentials *BuildCredentials `json:"credentials,omitempty"`
	// it is never stored
	Deploy *DeployContext `json:"deploy,omitempty"`
}

// DeployContext is what an Agent needs to run one deployment beyond its payload
type DeployContext struct {
	Registry *RegistryCredentials `json:"registry,omitempty"`
	Previous *PreviousRelease     `json:"previous,omitempty"`
	// they are never stored or logged
	Secrets map[string]string `json:"secrets,omitempty"`
}

// PreviousRelease is a release the Agent can restore
type PreviousRelease struct {
	ReleaseID uuid.UUID         `json:"release_id"`
	CommitSHA string            `json:"commit_sha"`
	ImageRef  string            `json:"image_ref"`
	Config    servicecfg.Config `json:"config"`
}

// BuildCredentials let a Runner fetch one repository and push its image
type BuildCredentials struct {
	Git      GitCredentials      `json:"git"`
	Registry RegistryCredentials `json:"registry"`
	Image    string              `json:"image"`
}

// RegistryCredentials authenticate docker login to one registry
type RegistryCredentials struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// GitCredentials authenticate an HTTPS clone with HTTP Basic auth
type GitCredentials struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// LeaseRequest renews the lease on a claimed job attempt
type LeaseRequest struct {
	Attempt int32 `json:"attempt"`
}

// LeaseResponse carries the new lease expiry
type LeaseResponse struct {
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}

// Log streams accepted in job events
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
	StreamSystem = "system"
)

// Batch limits for job events
const (
	MaxEventsPerBatch = 500
	MaxEventBytes     = 16 * 1024
)

// EventsRequest appends log lines for a job attempt
type EventsRequest struct {
	Attempt int32      `json:"attempt"`
	Events  []LogEvent `json:"events"`
}

// LogEvent is one line of job output
type LogEvent struct {
	Sequence int64  `json:"sequence"`
	Stream   string `json:"stream"`
	Message  string `json:"message"`
}

// Outcomes an Agent may report for a job attempt
const (
	OutcomeSucceeded = "succeeded"
	OutcomeFailed    = "failed"
)

// MaxResultBytes bounds the structured result stored with a job
const MaxResultBytes = 64 * 1024

// CompleteRequest reports the final outcome of an attempt
type CompleteRequest struct {
	Attempt      int32           `json:"attempt"`
	Outcome      string          `json:"outcome"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorCode    string          `json:"error_code,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
}

// Job types
const (
	JobContainerAction = "container_action"
	JobFetchLogs       = "fetch_logs"
	JobDeployImage     = "deploy_image"
	JobBuild           = "build"
)

// BuildResult is a successful build's job result
type BuildResult struct {
	CommitSHA string `json:"commit_sha"`
	Image     string `json:"image"`     // e.g. ghcr.io/acme/shop-api
	Tag       string `json:"tag"`       // short commit SHA
	Digest    string `json:"digest"`    // sha256:…
	ImageRef  string `json:"image_ref"` // Image@Digest
}

// Stages of a deployment
const (
	StagePull        = "pull"
	StageStart       = "start"
	StageHealthCheck = "health_check"
)

// StageLine formats a system log line that belongs to a deployment stage
func StageLine(stage, message string) string {
	return "[" + stage + "] " + message
}

// StageOfLine returns the stage a StageLine belongs to, or "" for other lines
func StageOfLine(line string) string {
	for _, stage := range []string{StagePull, StageStart, StageHealthCheck} {
		if strings.HasPrefix(line, "["+stage+"] ") {
			return stage
		}
	}
	return ""
}

// DeployResult is a successful deploy_image job's result
type DeployResult struct {
	ImageRef    string `json:"image_ref"`
	ContainerID string `json:"container_id"`
}

// DeployImagePayload deploys one image digest to one environment on the Agent's server
type DeployImagePayload struct {
	DeploymentID  uuid.UUID         `json:"deployment_id"`
	EnvironmentID uuid.UUID         `json:"environment_id"`
	ApplicationID uuid.UUID         `json:"application_id"`
	CommitSHA     string            `json:"commit_sha"`
	ImageRef      string            `json:"image_ref"` // registry/repo@sha256:…
	Config        servicecfg.Config `json:"config"`
}

// BuildPayload builds and pushes an image for one commit
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
	Trigger            string    `json:"trigger,omitempty"`
}

// Build triggers
const (
	TriggerPush   = "push"
	TriggerManual = "manual"
	TriggerCommit = "commit"
)

// Container actions accepted by JobContainerAction
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
)

// ContainerActionPayload starts, stops, or restarts one container
type ContainerActionPayload struct {
	ContainerID string `json:"container_id"`
	Action      string `json:"action"`
}

// Log fetch limits
const (
	DefaultTailLines = 500
	MaxTailLines     = 2000
	MaxFetchLogBytes = 1 << 20
)

// FetchLogsPayload reads a bounded range of a container's past output
type FetchLogsPayload struct {
	ContainerID string     `json:"container_id"`
	TailLines   int        `json:"tail_lines"`
	Since       *time.Time `json:"since,omitempty"`
	Timestamps  bool       `json:"timestamps"`
}
