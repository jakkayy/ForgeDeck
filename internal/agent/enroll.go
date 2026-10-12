package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
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

	resp, err := NewClient(controlPlaneURL, "").Enroll(ctx, token, version)
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

// ValidateControlPlaneURL requires HTTPS so the credential is never sent in
// clear text, except to a loopback address or when allowInsecure is set.
func ValidateControlPlaneURL(raw string, allowInsecure bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("control plane URL %q must look like https://forgedeck.example.com", raw)
	}
	if u.Scheme == "https" || allowInsecure || isLoopback(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("control plane URL %q is not HTTPS; use https or pass --allow-insecure for a trusted network", raw)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
