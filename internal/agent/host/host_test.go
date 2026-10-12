package host

import (
	"runtime"
	"testing"
)

func TestBusyPercentSince(t *testing.T) {
	prev := cpuTimes{idle: 800, total: 1000}
	cases := []struct {
		now  cpuTimes
		want float32
	}{
		{cpuTimes{idle: 850, total: 1100}, 50},
		{cpuTimes{idle: 900, total: 1100}, 0},
		{cpuTimes{idle: 800, total: 1100}, 100},
		{prev, 0},
	}
	for _, tc := range cases {
		if got := tc.now.busyPercentSince(prev); got != tc.want {
			t.Errorf("%+v: got %v, want %v", tc.now, got, tc.want)
		}
	}
	if got := (cpuTimes{idle: 1, total: 2}).busyPercentSince(cpuTimes{}); got != 0 {
		t.Errorf("first sample: got %v, want 0", got)
	}
}

func TestSampleOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("host metrics are Linux-only")
	}
	s := NewSampler("/")
	s.Sample()
	r := s.Sample()
	if r == nil {
		t.Fatal("no sample on Linux")
	}
	for name, v := range map[string]float32{"cpu": r.CPUPercent, "memory": r.MemoryPercent, "disk": r.DiskPercent} {
		if v < 0 || v > 100 {
			t.Errorf("%s = %v, out of range", name, v)
		}
	}
	if r.MemoryPercent == 0 || r.DiskPercent == 0 {
		t.Errorf("implausible zero usage: %+v", r)
	}
}
