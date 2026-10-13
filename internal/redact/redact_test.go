package redact_test

import (
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/redact"
)

func TestNewMasksEveryOccurrenceLongestFirst(t *testing.T) {
	got := redact.New([]string{"hunter22", "hunter22-long", "hunter22"})("a hunter22 b hunter22-long c hunter22")
	if got != "a ******** b ******** c ********" {
		t.Fatalf("got %q", got)
	}
}

func TestNewIgnoresShortAndEmptySecrets(t *testing.T) {
	if got := redact.New([]string{"", "abc"})("abc def"); got != "abc def" {
		t.Fatalf("got %q", got)
	}
	if got := redact.New(nil)("unchanged"); got != "unchanged" {
		t.Fatalf("got %q", got)
	}
}
