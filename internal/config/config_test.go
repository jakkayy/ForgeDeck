package config

import (
	"strings"
	"testing"
)

func envOf(vars map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{
		"FORGEDECK_DATABASE_URL": "postgres://localhost/forgedeck",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		HTTPAddr:     ":8080",
		DatabaseURL:  "postgres://localhost/forgedeck",
		LogLevel:     "info",
		LogFormat:    "json",
		CookieSecure: true,
	}
	if cfg != want {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadReadsOverrides(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{
		"FORGEDECK_HTTP_ADDR":     "127.0.0.1:9000",
		"FORGEDECK_DATABASE_URL":  "postgres://db/forgedeck",
		"FORGEDECK_LOG_LEVEL":     "debug",
		"FORGEDECK_LOG_FORMAT":    "text",
		"FORGEDECK_COOKIE_SECURE": "false",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:9000" || cfg.LogLevel != "debug" || cfg.LogFormat != "text" || cfg.CookieSecure {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestLoadReportsAllInvalidValues(t *testing.T) {
	_, err := Load(envOf(map[string]string{
		"FORGEDECK_LOG_LEVEL":     "verbose",
		"FORGEDECK_LOG_FORMAT":    "xml",
		"FORGEDECK_COOKIE_SECURE": "maybe",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"FORGEDECK_COOKIE_SECURE", "FORGEDECK_LOG_LEVEL", "FORGEDECK_LOG_FORMAT", "FORGEDECK_DATABASE_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	_, err := Load(envOf(map[string]string{}))
	if err == nil || !strings.Contains(err.Error(), "FORGEDECK_DATABASE_URL") {
		t.Fatalf("expected missing database URL error, got %v", err)
	}
}
