package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jakkayy/ForgeDeck/internal/agent/docker"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// LogReader reads a container's past output.
type LogReader interface {
	Logs(ctx context.Context, id string, opts docker.LogOptions, stdout, stderr io.Writer) error
}

// FetchLogsResult summarizes what was returned.
type FetchLogsResult struct {
	Lines     int  `json:"lines"`
	Truncated bool `json:"truncated"`
}

// errLogBudget stops reading once the byte budget is used up.
var errLogBudget = errors.New("log byte budget exhausted")

// FetchLogsHandler streams a bounded range of past container output as job
// logs. Limits are enforced here regardless of what the payload asks for.
func FetchLogsHandler(engine LogReader) worker.Handler {
	return func(ctx context.Context, raw json.RawMessage, log worker.Logger) (any, error) {
		var p agentproto.FetchLogsPayload
		if err := json.Unmarshal(raw, &p); err != nil || !shortContainerID.MatchString(p.ContainerID) {
			return nil, errInvalidPayload
		}
		tail := p.TailLines
		if tail < 1 || tail > agentproto.MaxTailLines {
			tail = agentproto.DefaultTailLines
		}

		budget := &logBudget{remaining: agentproto.MaxFetchLogBytes}
		stdout := &lineWriter{stream: agentproto.StreamStdout, log: log, budget: budget}
		stderr := &lineWriter{stream: agentproto.StreamStderr, log: log, budget: budget}
		err := engine.Logs(ctx, p.ContainerID, docker.LogOptions{TailLines: tail, Since: p.Since, Timestamps: p.Timestamps}, stdout, stderr)
		stdout.flush()
		stderr.flush()

		switch {
		case errors.Is(err, docker.ErrContainerNotFound):
			return nil, errNoContainer
		case err != nil && !errors.Is(err, errLogBudget):
			return nil, &worker.JobError{Code: "docker_error", Message: err.Error()}
		}
		result := FetchLogsResult{Lines: budget.lines, Truncated: budget.exhausted}
		if result.Truncated {
			log.Log(agentproto.StreamSystem, fmt.Sprintf("Output truncated at %d bytes.", agentproto.MaxFetchLogBytes))
		}
		return result, nil
	}
}

// logBudget is shared by stdout and stderr so their total stays bounded.
type logBudget struct {
	remaining int
	lines     int
	exhausted bool
}

// lineWriter turns a byte stream into log lines.
type lineWriter struct {
	stream  string
	log     worker.Logger
	budget  *logBudget
	partial []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	if w.budget.exhausted {
		return 0, errLogBudget
	}
	if len(p) > w.budget.remaining {
		p = p[:w.budget.remaining]
		w.budget.exhausted = true
	}
	w.budget.remaining -= len(p)

	w.partial = append(w.partial, p...)
	for {
		i := bytes.IndexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		w.emit(w.partial[:i])
		w.partial = w.partial[i+1:]
	}
	if w.budget.exhausted {
		return len(p), errLogBudget
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if len(w.partial) > 0 {
		w.emit(w.partial)
		w.partial = nil
	}
}

func (w *lineWriter) emit(line []byte) {
	w.log.Log(w.stream, string(bytes.TrimSuffix(line, []byte("\r"))))
	w.budget.lines++
}
