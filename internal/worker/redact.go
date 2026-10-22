package worker

import "github.com/jakkayy/ForgeDeck/internal/redact"

// redactingLog masks known secrets in every line; it cannot catch a transformed secret
type redactingLog struct {
	next   Logger
	redact func(string) string
}

// NewRedactingLog returns a Logger that masks every secret with "********"
func NewRedactingLog(next Logger, secrets []string) Logger {
	return &redactingLog{next: next, redact: redact.New(secrets)}
}

func (l *redactingLog) Log(stream, message string) {
	l.next.Log(stream, l.redact(message))
}
