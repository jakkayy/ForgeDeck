// Package redact masks known secret values in text.
package redact

import (
	"cmp"
	"slices"
	"strings"
)

// Mask replaces a secret.
const Mask = "********"

// MinLength is the shortest value that is masked: shorter strings would also
// mask ordinary words.
const MinLength = 4

// New returns a function that replaces every occurrence of each secret with
// Mask. Secrets shorter than MinLength are ignored, and when one secret
// contains another the longer one wins. With nothing to mask the function
// returns its input.
func New(secrets []string) func(string) string {
	secrets = slices.DeleteFunc(slices.Clone(secrets), func(s string) bool { return len(s) < MinLength })
	if len(secrets) == 0 {
		return func(s string) string { return s }
	}
	// strings.Replacer tries patterns in argument order, so sort longest first.
	slices.SortFunc(secrets, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	pairs := make([]string, 0, 2*len(secrets))
	for _, secret := range slices.Compact(secrets) {
		pairs = append(pairs, secret, Mask)
	}
	return strings.NewReplacer(pairs...).Replace
}
