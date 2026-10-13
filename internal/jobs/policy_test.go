package jobs

import "testing"

func TestMaxAttempts(t *testing.T) {
	cases := []struct {
		jobType   string
		requested int32
		want      int32
	}{
		{"container_action", 0, 1},
		{"fetch_logs", 0, 2},
		{"build", 0, 2},
		{"unknown", 0, 1},
		{"container_action", 3, 3},
	}
	for _, tc := range cases {
		if got := maxAttempts(tc.jobType, tc.requested); got != tc.want {
			t.Errorf("maxAttempts(%q, %d) = %d, want %d", tc.jobType, tc.requested, got, tc.want)
		}
	}
}
