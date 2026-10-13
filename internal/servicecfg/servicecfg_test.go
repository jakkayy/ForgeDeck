package servicecfg

import (
	"errors"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/validate"
)

func ptr[T any](v T) *T { return &v }

func TestNormalizeAppliesSafeDefaults(t *testing.T) {
	got, err := Normalize(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if got.BindAddress != BindLoopback || got.Restart != RestartUnlessStopped ||
		got.HealthCheck != (HealthCheck{Type: HealthDocker, TimeoutSeconds: 60, IntervalSeconds: 2}) {
		t.Fatalf("defaults %+v", got)
	}

	got, err = Normalize(Config{ContainerPort: ptr(8080), PublishedPort: ptr(18080)})
	if err != nil || got.HealthCheck.Type != HealthTCP {
		t.Fatalf("published port default check %+v, %v", got.HealthCheck, err)
	}
}

func TestNormalizeCleansPathsAndDoesNotMutateInput(t *testing.T) {
	in := Config{Volumes: []Volume{{Name: "data", Path: "/var//lib/app/"}}, Env: map[string]string{"A": "1"}}
	got, err := Normalize(in)
	if err != nil || got.Volumes[0].Path != "/var/lib/app" {
		t.Fatalf("got %+v, %v", got.Volumes, err)
	}
	if in.Volumes[0].Path != "/var//lib/app/" {
		t.Fatal("input was modified")
	}
}

func TestNormalizeRejectsUnsafeConfig(t *testing.T) {
	cases := map[string]struct {
		cfg   Config
		field string
	}{
		"container port zero":      {Config{ContainerPort: ptr(0)}, "container_port"},
		"published without target": {Config{PublishedPort: ptr(8080)}, "published_port"},
		"privileged host port":     {Config{ContainerPort: ptr(80), PublishedPort: ptr(80)}, "published_port"},
		"bind to a hostname":       {Config{BindAddress: "example.com"}, "bind_address"},
		"bad env name":             {Config{Env: map[string]string{"A-B": "x"}}, "env"},
		"nul in env value":         {Config{Env: map[string]string{"A": "x\x00"}}, "env"},
		"host path volume":         {Config{Volumes: []Volume{{Name: "/var/run/docker.sock", Path: "/sock"}}}, "volumes"},
		"relative mount path":      {Config{Volumes: []Volume{{Name: "data", Path: "data"}}}, "volumes"},
		"mount path traversal":     {Config{Volumes: []Volume{{Name: "data", Path: "/a/../etc"}}}, "volumes"},
		"mount root":               {Config{Volumes: []Volume{{Name: "data", Path: "/"}}}, "volumes"},
		"duplicate volume":         {Config{Volumes: []Volume{{Name: "d", Path: "/a"}, {Name: "d", Path: "/b"}}}, "volumes"},
		"unknown restart":          {Config{Restart: "sometimes"}, "restart"},
		"tiny memory":              {Config{MemoryMB: ptr(1)}, "memory_mb"},
		"huge cpus":                {Config{CPUs: ptr(1000.0)}, "cpus"},
		"http without port":        {Config{HealthCheck: HealthCheck{Type: HealthHTTP, Path: "/"}}, "health_check.type"},
		"http bad path":            {Config{ContainerPort: ptr(80), PublishedPort: ptr(8080), HealthCheck: HealthCheck{Type: HealthHTTP, Path: "healthz"}}, "health_check.path"},
		"http path with space":     {Config{ContainerPort: ptr(80), PublishedPort: ptr(8080), HealthCheck: HealthCheck{Type: HealthHTTP, Path: "/a b"}}, "health_check.path"},
		"protocol relative path":   {Config{ContainerPort: ptr(80), PublishedPort: ptr(8080), HealthCheck: HealthCheck{Type: HealthHTTP, Path: "//evil.example"}}, "health_check.path"},
		"unknown check":            {Config{HealthCheck: HealthCheck{Type: "exec"}}, "health_check.type"},
		"timeout too long":         {Config{HealthCheck: HealthCheck{TimeoutSeconds: 601}}, "health_check.timeout_seconds"},
		"interval above timeout":   {Config{HealthCheck: HealthCheck{TimeoutSeconds: 5, IntervalSeconds: 10}}, "health_check.interval_seconds"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Normalize(tc.cfg)
			var errs validate.Errors
			if !errors.As(err, &errs) || errs[tc.field] == "" {
				t.Fatalf("got %v, want an error on %s", err, tc.field)
			}
		})
	}
}

func TestNormalizeAcceptsFullConfig(t *testing.T) {
	_, err := Normalize(Config{
		ContainerPort: ptr(8080), PublishedPort: ptr(18080), BindAddress: BindAll,
		Env:     map[string]string{"LOG_LEVEL": "info"},
		Volumes: []Volume{{Name: "app-data", Path: "/var/lib/app"}},
		Restart: RestartAlways, MemoryMB: ptr(512), CPUs: ptr(1.5),
		HealthCheck: HealthCheck{Type: HealthHTTP, Path: "/healthz?full=1", TimeoutSeconds: 30, IntervalSeconds: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
}
