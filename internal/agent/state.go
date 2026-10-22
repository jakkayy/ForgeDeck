// Package agent implements the ForgeDeck Agent
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// DefaultStateDir holds the Agent's identity on a Linux host
const DefaultStateDir = "/var/lib/forgedeck-agent"

const identityFile = "identity.json"

// ErrNotEnrolled means no credential is stored yet
var ErrNotEnrolled = errors.New("agent is not enrolled: run `forgedeck-agent enroll` first")

// Identity is what the Agent keeps after enrolling
type Identity struct {
	ControlPlaneURL string    `json:"control_plane_url"`
	AgentID         uuid.UUID `json:"agent_id"`
	ServerID        uuid.UUID `json:"server_id"`
	Credential      string    `json:"credential"`
}

// State reads and writes the Agent identity in a private directory
type State struct {
	dir string
}

// NewState uses dir, creating it with owner-only permissions if needed
func NewState(dir string) (*State, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // directories need the execute bit
		return nil, fmt.Errorf("restrict state dir: %w", err)
	}
	return &State{dir: dir}, nil
}

// Load returns the stored identity or ErrNotEnrolled
func (s *State) Load() (Identity, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, identityFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Identity{}, ErrNotEnrolled
	}
	if err != nil {
		return Identity{}, fmt.Errorf("read credential: %w", err)
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return Identity{}, fmt.Errorf("parse credential file: %w", err)
	}
	return id, nil
}

// Save stores id atomically with owner-only permissions
func (s *State) Save(id Identity) error {
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credential: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, identityFile+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp credential: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("restrict credential: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write credential: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync credential: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close credential: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(s.dir, identityFile)); err != nil {
		return fmt.Errorf("store credential: %w", err)
	}
	return nil
}
