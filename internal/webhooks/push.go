// Package webhooks receives Git provider webhooks: it verifies them,
// drops redeliveries, and turns pushes into build jobs.
package webhooks

import (
	"regexp"
	"strings"
)

// Push is a provider-neutral branch push.
type Push struct {
	// RepositoryID is the provider's repository ID, as stored on applications.
	RepositoryID string
	Branch       string
	CommitSHA    string
	// CommitMessage is the head commit's first line, for display.
	CommitMessage string
}

var validSHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// branchFromRef returns the branch of a "refs/heads/<branch>" ref; tags and
// other refs are not builds.
func branchFromRef(ref string) (string, bool) {
	branch, ok := strings.CutPrefix(ref, "refs/heads/")
	return branch, ok && branch != ""
}

// firstLine trims a commit message to its subject, bounded for storage.
func firstLine(message string) string {
	subject, _, _ := strings.Cut(message, "\n")
	if len(subject) > 200 {
		subject = subject[:200]
	}
	return subject
}

// isDeletion reports the all-zero SHA providers send when a branch is deleted.
func isDeletion(sha string) bool {
	return strings.Trim(sha, "0") == ""
}
