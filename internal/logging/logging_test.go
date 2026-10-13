package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewJSONRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(&buf, "warn", "json")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	logger.Info("hidden")
	logger.Warn("shown", "event", "test.event")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d: %q", len(lines), buf.String())
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if entry["msg"] != "shown" || entry["event"] != "test.event" {
		t.Fatalf("unexpected entry: %v", entry)
	}
}

func TestNewRejectsInvalidSettings(t *testing.T) {
	if _, err := New(&bytes.Buffer{}, "loud", "json"); err == nil {
		t.Error("expected error for invalid level")
	}
	if _, err := New(&bytes.Buffer{}, "info", "xml"); err == nil {
		t.Error("expected error for invalid format")
	}
}
