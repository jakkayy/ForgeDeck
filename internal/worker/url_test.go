package worker_test

import (
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/worker"
)

func TestValidateControlPlaneURL(t *testing.T) {
	cases := []struct {
		url           string
		allowInsecure bool
		ok            bool
	}{
		{"https://forgedeck.example.com", false, true},
		{"http://forgedeck.example.com", false, false},
		{"http://forgedeck.example.com", true, true},
		{"http://127.0.0.1:8080", false, true},
		{"http://localhost:8080", false, true},
		{"forgedeck.example.com", false, false},
		{"ftp://forgedeck.example.com", true, false},
	}
	for _, tc := range cases {
		err := worker.ValidateControlPlaneURL(tc.url, tc.allowInsecure)
		if (err == nil) != tc.ok {
			t.Errorf("%s (insecure=%v): err=%v, want ok=%v", tc.url, tc.allowInsecure, err, tc.ok)
		}
	}
}
