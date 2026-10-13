package securetoken

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	token, hash := New("fds_")
	if !strings.HasPrefix(token, "fds_") || len(token) < 40 {
		t.Fatalf("unexpected token %q", token)
	}
	if !bytes.Equal(hash, Hash(token)) {
		t.Fatal("returned hash does not match Hash(token)")
	}
	other, _ := New("fds_")
	if other == token {
		t.Fatal("tokens repeat")
	}
}
