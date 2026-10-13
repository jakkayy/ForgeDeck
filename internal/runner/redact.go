package runner

import (
	"cmp"
	"encoding/base64"
	"slices"
	"strings"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

const (
	redacted = "********"
	// minSecretLength avoids masking common short strings that happen to
	// equal a weak password.
	minSecretLength = 4
)

// redactingLog replaces known secret values in every line before it leaves
// the Runner. It catches what build tools echo back; it cannot catch a
// secret that a tool transforms (for example re-encodes) first.
type redactingLog struct {
	next     worker.Logger
	replacer *strings.Replacer
}

func newRedactingLog(next worker.Logger, secrets []string) worker.Logger {
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

// secretsOf lists the values of credentials that must never appear in a
// build log, including the HTTP Basic forms git and docker send.
func secretsOf(c agentproto.BuildCredentials) []string {
	return []string{
		c.Git.Password,
		basicAuth(c.Git.Username, c.Git.Password),
		c.Registry.Password,
		basicAuth(c.Registry.Username, c.Registry.Password),
	}
}

func basicAuth(username, password string) string {
	if password == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}
