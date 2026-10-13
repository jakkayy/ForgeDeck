package config

import (
	"strings"
	"testing"
)

// testKey is a valid keyring spec; tests that are not about it include it.
const testKey = "v1:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

func envOf(vars map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{
		"FORGEDECK_DATABASE_URL": "postgres://localhost/forgedeck",
		"FORGEDECK_SECRET_KEYS":  testKey,
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.LogLevel != "info" || cfg.LogFormat != "json" || !cfg.CookieSecure || cfg.Keyring == nil {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
}

func TestLoadReadsOverrides(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{
		"FORGEDECK_HTTP_ADDR":     "127.0.0.1:9000",
		"FORGEDECK_DATABASE_URL":  "postgres://db/forgedeck",
		"FORGEDECK_LOG_LEVEL":     "debug",
		"FORGEDECK_LOG_FORMAT":    "text",
		"FORGEDECK_COOKIE_SECURE": "false",
		"FORGEDECK_SECRET_KEYS":   testKey,
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
	for _, want := range []string{"FORGEDECK_COOKIE_SECURE", "FORGEDECK_LOG_LEVEL", "FORGEDECK_LOG_FORMAT", "FORGEDECK_DATABASE_URL", "FORGEDECK_SECRET_KEYS"} {
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

func TestLoadGitHubAppIsOptionalButComplete(t *testing.T) {
	base := map[string]string{"FORGEDECK_DATABASE_URL": "postgres://db/f", "FORGEDECK_SECRET_KEYS": testKey}

	cfg, err := Load(envOf(base))
	if err != nil || cfg.GitHub != nil {
		t.Fatalf("without App ID: github=%v err=%v", cfg.GitHub, err)
	}

	partial := map[string]string{"FORGEDECK_GITHUB_APP_ID": "123"}
	for k, v := range base {
		partial[k] = v
	}
	_, err = Load(envOf(partial))
	for _, want := range []string{"FORGEDECK_GITHUB_APP_SLUG", "FORGEDECK_GITHUB_CLIENT_SECRET", "FORGEDECK_GITHUB_WEBHOOK_SECRET", "FORGEDECK_GITHUB_APP_PRIVATE_KEY", "FORGEDECK_PUBLIC_URL"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("partial GitHub config: error %v does not mention %s", err, want)
		}
	}
}
