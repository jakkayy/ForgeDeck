package agent

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agent/docker"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

type fakeLogs struct {
	stdout, stderr string
	err            error
	gotTail        int
}

func (f *fakeLogs) Logs(_ context.Context, _ string, opts docker.LogOptions, stdout, stderr io.Writer) error {
	f.gotTail = opts.TailLines
	if f.err != nil {
		return f.err
	}
	if _, err := io.WriteString(stdout, f.stdout); err != nil {
		return err
	}
	_, err := io.WriteString(stderr, f.stderr)
	return err
}

type streamLog struct{ lines []string }

func (s *streamLog) Log(stream, message string) { s.lines = append(s.lines, stream+": "+message) }

func fetch(t *testing.T, engine *fakeLogs, payload agentproto.FetchLogsPayload) (FetchLogsResult, *streamLog, error) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	log := &streamLog{}
	result, err := FetchLogsHandler(engine)(context.Background(), &agentproto.Job{Payload: raw}, log)
	if err != nil {
		return FetchLogsResult{}, log, err
	}
	return result.(FetchLogsResult), log, nil
}

func TestFetchLogsSplitsStreamsIntoLines(t *testing.T) {
	engine := &fakeLogs{stdout: "listening on :8080\r\nGET /health 200\npartial", stderr: "warning: slow query\n"}
	result, log, err := fetch(t, engine, agentproto.FetchLogsPayload{ContainerID: apiID, TailLines: 50})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"stdout: listening on :8080",
		"stdout: GET /health 200",
		"stderr: warning: slow query",
		"stdout: partial",
	}
	if strings.Join(log.lines, "|") != strings.Join(want, "|") || result.Lines != 4 || result.Truncated {
		t.Fatalf("lines %q result %+v", log.lines, result)
	}
	if engine.gotTail != 50 {
		t.Fatalf("tail %d", engine.gotTail)
	}
}

func TestFetchLogsEnforcesLimits(t *testing.T) {
	engine := &fakeLogs{stdout: strings.Repeat(strings.Repeat("x", 99)+"\n", agentproto.MaxFetchLogBytes/100+50)}
	result, log, err := fetch(t, engine, agentproto.FetchLogsPayload{ContainerID: apiID, TailLines: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || engine.gotTail != agentproto.DefaultTailLines {
		t.Fatalf("result %+v tail %d", result, engine.gotTail)
	}
	if last := log.lines[len(log.lines)-1]; !strings.Contains(last, "truncated") {
		t.Fatalf("no truncation notice: %q", last)
	}
}

func TestFetchLogsMissingContainer(t *testing.T) {
	_, _, err := fetch(t, &fakeLogs{err: docker.ErrContainerNotFound}, agentproto.FetchLogsPayload{ContainerID: apiID})
	if jobErrorCode(err) != "container_not_found" {
		t.Fatalf("got %v", err)
	}
}
