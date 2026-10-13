package worker

import "github.com/jakkayy/ForgeDeck/internal/redact"

// redactingLog replaces known secret values in every line before it leaves
// the worker. It catches what tools echo back; it cannot catch a secret that
// a tool transforms (for example re-encodes) first.
type redactingLog struct {
	next   Logger
	redact func(string) string
}

// NewRedactingLog returns a Logger that replaces every secret in what it
// logs with "********". Secrets shorter than four bytes are ignored.
func NewRedactingLog(next Logger, secrets []string) Logger {
	return &redactingLog{next: next, redact: redact.New(secrets)}
}

func (l *redactingLog) Log(stream, message string) {
	l.next.Log(stream, l.redact(message))
}
