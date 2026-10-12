package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/jakkayy/ForgeDeck/internal/agent/docker"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

// ContainerOperator performs container lifecycle actions.
type ContainerOperator interface {
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
	Restart(ctx context.Context, id string) error
}

var (
	errInvalidPayload = &JobError{Code: "invalid_payload", Message: "The job payload is not valid for this Agent."}
	errSelfAction     = &JobError{Code: "agent_container", Message: "Refusing to act on the Agent's own container; it would disconnect this server."}
	errNoContainer    = &JobError{Code: "container_not_found", Message: "The container no longer exists on this server."}
	shortContainerID  = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
)

// ContainerActionHandler starts, stops, or restarts a container. selfID is
// the Agent's own container ID ("" when it runs outside a container).
func ContainerActionHandler(engine ContainerOperator, selfID string) Handler {
	actions := map[string]func(context.Context, string) error{
		agentproto.ActionStart:   engine.Start,
		agentproto.ActionStop:    engine.Stop,
		agentproto.ActionRestart: engine.Restart,
	}
	return func(ctx context.Context, raw json.RawMessage, log Logger) (any, error) {
		var p agentproto.ContainerActionPayload
		if err := json.Unmarshal(raw, &p); err != nil || !shortContainerID.MatchString(p.ContainerID) {
			return nil, errInvalidPayload
		}
		apply, ok := actions[p.Action]
		if !ok {
			return nil, errInvalidPayload
		}
		if isSelf(p.ContainerID, selfID) {
			return nil, errSelfAction
		}

		log.Log(agentproto.StreamSystem, fmt.Sprintf("%s container %s", p.Action, p.ContainerID[:12]))
		if err := apply(ctx, p.ContainerID); err != nil {
			if errors.Is(err, docker.ErrContainerNotFound) {
				return nil, errNoContainer
			}
			return nil, &JobError{Code: "docker_error", Message: err.Error()}
		}
		log.Log(agentproto.StreamSystem, "done")
		return p, nil
	}
}

// SelfContainerID returns the Agent's container ID when it runs in Docker:
// FORGEDECK_AGENT_CONTAINER_ID if set, otherwise Docker's default hostname,
// which is the short container ID.
func SelfContainerID() string {
	if id := os.Getenv("FORGEDECK_AGENT_CONTAINER_ID"); id != "" {
		return id
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		return ""
	}
	hostname, err := os.Hostname()
	if err != nil || !shortContainerID.MatchString(hostname) {
		return ""
	}
	return hostname
}

func isSelf(containerID, selfID string) bool {
	if selfID == "" {
		return false
	}
	return strings.HasPrefix(containerID, selfID) || strings.HasPrefix(selfID, containerID)
}
