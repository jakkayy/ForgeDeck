// Package validate collects field-level validation failures in a transport-
// neutral form, so services can reject input without knowing about HTTP.
package validate

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Errors maps a field name to a human-readable message.
type Errors map[string]string

// Add records a message for field, keeping the first one if called twice.
func (e Errors) Add(field, message string) {
	if _, exists := e[field]; !exists {
		e[field] = message
	}
}

// Err returns e as an error, or nil when no field failed.
func (e Errors) Err() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

func (e Errors) Error() string {
	parts := make([]string, 0, len(e))
	for _, field := range slices.Sorted(maps.Keys(e)) {
		parts = append(parts, fmt.Sprintf("%s: %s", field, e[field]))
	}
	return "validation failed: " + strings.Join(parts, "; ")
}
