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
