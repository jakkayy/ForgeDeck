package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// Enroll registers this host with the Control Plane and stores the identity.
// It refuses to replace an existing identity: remove the server in the
// dashboard and delete the state directory to re-enroll.
func Enroll(ctx context.Context, state *State, controlPlaneURL, token, version string) (Identity, error) {
	if _, err := state.Load(); err == nil {
		return Identity{}, errors.New("agent is already enrolled; delete the state directory to enroll again")
	} else if !errors.Is(err, ErrNotEnrolled) {
		return Identity{}, err
	}

	var resp agentproto.EnrollResponse
	err := worker.NewClient(controlPlaneURL, worker.AgentAPI, "").Post(ctx, "/enroll", agentproto.EnrollRequest{
		Token: token, AgentVersion: version, ProtocolVersion: agentproto.ProtocolVersion,
	}, &resp)
	if err != nil {
		return Identity{}, fmt.Errorf("enroll: %w", err)
	}
	identity := Identity{
		ControlPlaneURL: controlPlaneURL,
		AgentID:         resp.AgentID,
		ServerID:        resp.ServerID,
		Credential:      resp.Credential,
	}
	if err := state.Save(identity); err != nil {
		return Identity{}, err
	}
	return identity, nil
}
