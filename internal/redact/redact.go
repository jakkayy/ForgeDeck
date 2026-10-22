// Package redact masks known secret values in text.
package redact

import (
	"cmp"
	"slices"
	"strings"
)

const Mask = "********"

// MinLength is the shortest masked value; shorter strings would mask ordinary words
const MinLength = 4

// New returns a function that masks every secret (longest wins); secrets shorter than MinLength are ignored
func New(secrets []string) func(string) string {
	secrets = slices.DeleteFunc(slices.Clone(secrets), func(s string) bool { return len(s) < MinLength })
	if len(secrets) == 0 {
		return func(s string) string { return s }
	}
	// Replacer tries patterns in argument order, so sort longest first
	slices.SortFunc(secrets, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	pairs := make([]string, 0, 2*len(secrets))
	for _, secret := range slices.Compact(secrets) {
		pairs = append(pairs, secret, Mask)
	}
	return strings.NewReplacer(pairs...).Replace
}
