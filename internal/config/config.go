// Package config loads Control Plane settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/jakkayy/ForgeDeck/internal/retention"

	"github.com/jakkayy/ForgeDeck/internal/gitprovider/github"
	"github.com/jakkayy/ForgeDeck/internal/keyring"
	"github.com/jakkayy/ForgeDeck/internal/safehttp"
)

// Config holds the Control Plane runtime settings.
type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	LogLevel     string
	LogFormat    string
	CookieSecure bool
	// DisableWebSetup stops the sign-in page from offering to create the first
	// account; it is then created with `forgedeck-api create-user`.
	DisableWebSetup bool
	// Keyring encrypts stored credentials; parsed from FORGEDECK_SECRET_KEYS.
	Keyring *keyring.Keyring
	// PublicURL is where users and Git providers reach ForgeDeck, used for
	// webhook and OAuth callback URLs. Optional until a provider needs it.
	PublicURL string
	// GitHub is nil unless the ForgeDeck GitHub App is configured.
	GitHub *github.Config
	// GitHubWebhookSecret verifies GitHub App webhook deliveries.
	GitHubWebhookSecret string
	// TrustedProxies are the reverse proxies whose X-Forwarded-For header is
	// believed when finding a client's address (FORGEDECK_TRUSTED_PROXIES).
	TrustedProxies []netip.Prefix
	// Retention says how long old data is kept; zero keeps a kind forever.
	Retention retention.Policy
	// Outbound limits requests to user-supplied URLs such as a self-hosted
	// GitLab. FORGEDECK_ALLOW_PRIVATE_GIT_HOSTS opens private networks.
	Outbound safehttp.Policy
}

// LookupFunc matches os.LookupEnv so tests can supply their own environment.
type LookupFunc func(key string) (string, bool)

// Load reads FORGEDECK_* variables, applies defaults, and validates the result.
func Load(lookup LookupFunc) (Config, error) {
	env := reader{lookup: lookup}
	cfg := Config{
		HTTPAddr:        env.string("FORGEDECK_HTTP_ADDR", ":8080"),
		DatabaseURL:     env.string("FORGEDECK_DATABASE_URL", ""),
		LogLevel:        env.string("FORGEDECK_LOG_LEVEL", "info"),
		LogFormat:       env.string("FORGEDECK_LOG_FORMAT", "json"),
		CookieSecure:    env.bool("FORGEDECK_COOKIE_SECURE", true),
		DisableWebSetup: env.bool("FORGEDECK_DISABLE_WEB_SETUP", false),
	}
	keys, err := keyring.Parse(env.string("FORGEDECK_SECRET_KEYS", ""))
	if err != nil {
		env.errs = append(env.errs, fmt.Errorf("FORGEDECK_SECRET_KEYS: %w (generate a key with: openssl rand -base64 32)", err))
	}
	cfg.Keyring = keys
	cfg.PublicURL = strings.TrimRight(env.string("FORGEDECK_PUBLIC_URL", ""), "/")
	cfg.GitHub, cfg.GitHubWebhookSecret = env.gitHubApp()
	cfg.TrustedProxies = env.prefixes("FORGEDECK_TRUSTED_PROXIES")
	cfg.Retention = retention.Policy{
		JobLogDays:  env.days("FORGEDECK_RETENTION_JOB_LOGS_DAYS", retention.DefaultPolicy.JobLogDays),
		JobDays:     env.days("FORGEDECK_RETENTION_JOBS_DAYS", retention.DefaultPolicy.JobDays),
		WebhookDays: env.days("FORGEDECK_RETENTION_WEBHOOKS_DAYS", retention.DefaultPolicy.WebhookDays),
		AuditDays:   env.days("FORGEDECK_RETENTION_AUDIT_DAYS", retention.DefaultPolicy.AuditDays),
	}
	if err := cfg.Retention.Validate(); err != nil {
		env.errs = append(env.errs, fmt.Errorf("FORGEDECK_RETENTION_*: %w", err))
	}
	cfg.Outbound = safehttp.Policy{AllowPrivate: env.bool("FORGEDECK_ALLOW_PRIVATE_GIT_HOSTS", false)}

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
	if c.PublicURL != "" && !strings.HasPrefix(c.PublicURL, "https://") && !strings.HasPrefix(c.PublicURL, "http://") {
		errs = append(errs, fmt.Errorf("FORGEDECK_PUBLIC_URL %q must start with https://", c.PublicURL))
	}
	if c.GitHub != nil && c.PublicURL == "" {
		errs = append(errs, errors.New("FORGEDECK_PUBLIC_URL is required when the GitHub App is configured"))
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

// gitHubApp reads the optional GitHub App settings. Setting the App ID turns
// the integration on and makes the remaining settings required.
func (r *reader) gitHubApp() (*github.Config, string) {
	rawID := r.string("FORGEDECK_GITHUB_APP_ID", "")
	if rawID == "" {
		return nil, ""
	}
	appID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("FORGEDECK_GITHUB_APP_ID: %q is not a number", rawID))
	}
	cfg := &github.Config{
		AppID:        appID,
		Slug:         r.required("FORGEDECK_GITHUB_APP_SLUG"),
		ClientID:     r.required("FORGEDECK_GITHUB_CLIENT_ID"),
		ClientSecret: r.required("FORGEDECK_GITHUB_CLIENT_SECRET"),
		WebURL:       r.string("FORGEDECK_GITHUB_URL", "https://github.com"),
		APIURL:       r.string("FORGEDECK_GITHUB_API_URL", "https://api.github.com"),
	}
	webhookSecret := r.required("FORGEDECK_GITHUB_WEBHOOK_SECRET")

	pemData, err := r.secretValue("FORGEDECK_GITHUB_APP_PRIVATE_KEY")
	if err != nil {
		r.errs = append(r.errs, err)
		return cfg, webhookSecret
	}
	if cfg.PrivateKey, err = github.ParsePrivateKey(pemData); err != nil {
		r.errs = append(r.errs, fmt.Errorf("FORGEDECK_GITHUB_APP_PRIVATE_KEY: %w", err))
	}
	return cfg, webhookSecret
}

// secretValue reads KEY, or the file named by KEY_FILE, which keeps
// multi-line secrets such as PEM keys out of the environment.
func (r *reader) secretValue(key string) ([]byte, error) {
	if path := r.string(key+"_FILE", ""); path != "" {
		data, err := os.ReadFile(path) //nolint:gosec // path comes from operator configuration
		if err != nil {
			return nil, fmt.Errorf("%s_FILE: %w", key, err)
		}
		return data, nil
	}
	if value := r.string(key, ""); value != "" {
		return []byte(value), nil
	}
	return nil, fmt.Errorf("%s or %s_FILE is required", key, key)
}

func (r *reader) required(key string) string {
	value := r.string(key, "")
	if value == "" {
		r.errs = append(r.errs, fmt.Errorf("%s is required when the GitHub App is configured", key))
	}
	return value
}

// days reads a whole number of days, 0 meaning "keep forever".
func (r *reader) days(key string, fallback int) int {
	raw, ok := r.lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		r.errs = append(r.errs, fmt.Errorf("%s: %q must be a number of days, 0 or more (0 keeps it forever)", key, raw))
		return fallback
	}
	return n
}

// prefixes reads a comma-separated list of addresses or CIDR ranges.
func (r *reader) prefixes(key string) []netip.Prefix {
	raw, ok := r.lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []netip.Prefix
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			addr, addrErr := netip.ParseAddr(item)
			if addrErr != nil {
				r.errs = append(r.errs, fmt.Errorf("%s: %q is not an IP address or CIDR range", key, item))
				continue
			}
			prefix = netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen())
		}
		out = append(out, prefix.Masked())
	}
	return out
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
