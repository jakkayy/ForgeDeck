package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agent/docker"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

type recordingEngine struct {
	calls []string
	err   error
}

func (e *recordingEngine) record(action, id string) error {
	e.calls = append(e.calls, action+" "+id)
	return e.err
}
func (e *recordingEngine) Start(_ context.Context, id string) error   { return e.record("start", id) }
func (e *recordingEngine) Stop(_ context.Context, id string) error    { return e.record("stop", id) }
func (e *recordingEngine) Restart(_ context.Context, id string) error { return e.record("restart", id) }

type discardLog struct{ lines []string }

func (d *discardLog) Log(_, message string) { d.lines = append(d.lines, message) }

const apiID = "aaaaaaaaaaaa1111"

func runAction(t *testing.T, engine *recordingEngine, selfID string, payload map[string]string) error {
	t.Helper()
	raw, _ := json.Marshal(payload)
	_, err := ContainerActionHandler(engine, selfID)(context.Background(), raw, &discardLog{})
	return err
}

func jobErrorCode(err error) string {
	var jobErr *worker.JobError
	if errors.As(err, &jobErr) {
		return jobErr.Code
	}
	return ""
}

func TestContainerActionHandlerAppliesAction(t *testing.T) {
	for _, action := range []string{"start", "stop", "restart"} {
		engine := &recordingEngine{}
		if err := runAction(t, engine, "", map[string]string{"container_id": apiID, "action": action}); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if len(engine.calls) != 1 || engine.calls[0] != action+" "+apiID {
			t.Fatalf("%s: calls %v", action, engine.calls)
		}
	}
}

func TestContainerActionHandlerFailures(t *testing.T) {
	cases := []struct {
		name    string
		engine  *recordingEngine
		selfID  string
		payload map[string]string
		code    string
	}{
		{"unknown action", &recordingEngine{}, "", map[string]string{"container_id": apiID, "action": "kill"}, "invalid_payload"},
		{"bad container id", &recordingEngine{}, "", map[string]string{"container_id": "../x", "action": "stop"}, "invalid_payload"},
		{"own container", &recordingEngine{}, "aaaaaaaaaaaa", map[string]string{"container_id": apiID, "action": "stop"}, "agent_container"},
		{"vanished container", &recordingEngine{err: docker.ErrContainerNotFound}, "", map[string]string{"container_id": apiID, "action": "stop"}, "container_not_found"},
		{"docker failure", &recordingEngine{err: errors.New("daemon down")}, "", map[string]string{"container_id": apiID, "action": "stop"}, "docker_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := jobErrorCode(runAction(t, tc.engine, tc.selfID, tc.payload)); code != tc.code {
				t.Fatalf("code %q, want %q", code, tc.code)
			}
		})
	}
}
