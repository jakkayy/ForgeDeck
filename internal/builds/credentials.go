// Package builds connects build jobs to the services they depend on: it
// mints the credentials a Runner needs for one build attempt.
package builds

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
)

// Credentials mints build credentials at claim time.
type Credentials struct {
	git *gitconn.Service
}

// NewCredentials returns a Credentials minter.
func NewCredentials(git *gitconn.Service) *Credentials {
	return &Credentials{git: git}
}

// Mint returns the credentials for one attempt of the build described by
// payload.
func (c *Credentials) Mint(ctx context.Context, payload json.RawMessage) (*agentproto.BuildCredentials, error) {
	var build agentproto.BuildPayload
	if err := json.Unmarshal(payload, &build); err != nil {
		return nil, fmt.Errorf("decode build payload: %w", err)
	}
	git, err := c.git.CloneCredentials(ctx, build.GitConnectionID, build.RepositoryID, build.RepositoryFullName)
	if err != nil {
		return nil, fmt.Errorf("git credentials: %w", err)
	}
	return &agentproto.BuildCredentials{Git: git}, nil
}
