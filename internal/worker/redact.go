package worker

import (
	"cmp"
	"slices"
	"strings"
)

const (
	redacted = "********"
	// minSecretLength avoids masking common short strings that happen to
	// equal a weak password.
	minSecretLength = 4
)

// redactingLog replaces known secret values in every line before it leaves
// the worker. It catches what tools echo back; it cannot catch a secret that
// a tool transforms (for example re-encodes) first.
type redactingLog struct {
	next     Logger
	replacer *strings.Replacer
}

// NewRedactingLog returns a Logger that replaces every secret in what it
// logs with "********". Secrets shorter than four bytes are ignored.
func NewRedactingLog(next Logger, secrets []string) Logger {
	secrets = slices.DeleteFunc(slices.Clone(secrets), func(s string) bool { return len(s) < minSecretLength })
	if len(secrets) == 0 {
		return next
	}
	// strings.Replacer tries patterns in argument order, so the longest
	// secret wins when one contains another.
	slices.SortFunc(secrets, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	pairs := make([]string, 0, 2*len(secrets))
	for _, secret := range slices.Compact(secrets) {
		pairs = append(pairs, secret, redacted)
	}
	return &redactingLog{next: next, replacer: strings.NewReplacer(pairs...)}
}

func (l *redactingLog) Log(stream, message string) {
	l.next.Log(stream, l.replacer.Replace(message))
}
