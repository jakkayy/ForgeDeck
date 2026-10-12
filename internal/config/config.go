// Package config loads Control Plane settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jakkayy/ForgeDeck/internal/keyring"
)

// Config holds the Control Plane runtime settings.
type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	LogLevel     string
	LogFormat    string
	CookieSecure bool
	// Keyring encrypts stored credentials; parsed from FORGEDECK_SECRET_KEYS.
	Keyring *keyring.Keyring
}

// LookupFunc matches os.LookupEnv so tests can supply their own environment.
type LookupFunc func(key string) (string, bool)

// Load reads FORGEDECK_* variables, applies defaults, and validates the result.
func Load(lookup LookupFunc) (Config, error) {
	env := reader{lookup: lookup}
	cfg := Config{
		HTTPAddr:     env.string("FORGEDECK_HTTP_ADDR", ":8080"),
		DatabaseURL:  env.string("FORGEDECK_DATABASE_URL", ""),
		LogLevel:     env.string("FORGEDECK_LOG_LEVEL", "info"),
		LogFormat:    env.string("FORGEDECK_LOG_FORMAT", "json"),
		CookieSecure: env.bool("FORGEDECK_COOKIE_SECURE", true),
	}
	keys, err := keyring.Parse(env.string("FORGEDECK_SECRET_KEYS", ""))
	if err != nil {
		env.errs = append(env.errs, fmt.Errorf("FORGEDECK_SECRET_KEYS: %w (generate a key with: openssl rand -base64 32)", err))
	}
	cfg.Keyring = keys

	if err := errors.Join(append(env.errs, cfg.validate())...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("FORGEDECK_DATABASE_URL is required"))
	}
	if !slices.Contains([]string{"debug", "info", "warn", "error"}, c.LogLevel) {
		errs = append(errs, fmt.Errorf("FORGEDECK_LOG_LEVEL %q must be debug, info, warn or error", c.LogLevel))
	}
	if !slices.Contains([]string{"json", "text"}, c.LogFormat) {
		errs = append(errs, fmt.Errorf("FORGEDECK_LOG_FORMAT %q must be json or text", c.LogFormat))
	}
	return errors.Join(errs...)
}

type reader struct {
	lookup LookupFunc
	errs   []error
}

func (r *reader) string(key, fallback string) string {
	if v, ok := r.lookup(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func (r *reader) bool(key string, fallback bool) bool {
	raw := r.string(key, "")
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: %q is not a boolean", key, raw))
		return fallback
	}
	return v
}
