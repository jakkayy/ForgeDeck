package servers

import (
	"testing"
	"time"
)

func TestConnectivityStatus(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time {
		t := now.Add(-d)
		return &t
	}

	cases := []struct {
		name     string
		lastSeen *time.Time
		want     Status
	}{
		{"never seen", nil, StatusPending},
		{"just now", ago(0), StatusOnline},
		{"one missed heartbeat", ago(40 * time.Second), StatusOnline},
		{"three missed heartbeats", ago(61 * time.Second), StatusUnreachable},
		{"five minutes", ago(5 * time.Minute), StatusUnreachable},
		{"long gone", ago(5*time.Minute + time.Second), StatusOffline},
	}
	for _, tc := range cases {
		if got := ConnectivityStatus(tc.lastSeen, now); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}
