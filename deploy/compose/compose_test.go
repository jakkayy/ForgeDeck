// Package compose_test checks the production Compose file with `docker
// compose config`, which needs Docker but starts nothing. It runs only when
// FORGEDECK_TEST_DOCKER=1.
package compose_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/dockertest"
)

type service struct {
	Ports []struct {
		Published string `json:"published"`
	} `json:"ports"`
	User        string            `json:"user"`
	ReadOnly    bool              `json:"read_only"`
	CapDrop     []string          `json:"cap_drop"`
	SecurityOpt []string          `json:"security_opt"`
	Environment map[string]string `json:"environment"`
	Healthcheck *struct {
		Test []string `json:"test"`
	} `json:"healthcheck"`
	Networks map[string]struct {
		IPv4 string `json:"ipv4_address"`
	} `json:"networks"`
}

type config struct {
	Name     string             `json:"name"`
	Services map[string]service `json:"services"`
	Networks map[string]struct {
		Internal bool `json:"internal"`
	} `json:"networks"`
}

func render(t *testing.T, env map[string]string) (config, string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "docker", "compose", "--file", "docker-compose.yml", "--env-file", os.DevNull, "config", "--format", "json") //nolint:gosec // test
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	var cfg config
	if err == nil {
		if jsonErr := json.Unmarshal(out, &cfg); jsonErr != nil {
			t.Fatalf("%v\n%s", jsonErr, out)
		}
	}
	return cfg, string(out), err
}

var valid = map[string]string{
	"FORGEDECK_DOMAIN":      "forgedeck.example.com",
	"POSTGRES_PASSWORD":     "abc123",
	"FORGEDECK_SECRET_KEYS": "v1:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
}

func TestOnlyTheProxyPublishesPorts(t *testing.T) {
	dockertest.Require(t)
	cfg, out, err := render(t, valid)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, svc := range cfg.Services {
		if name != "proxy" && len(svc.Ports) > 0 {
			t.Errorf("%s publishes ports %+v: only the proxy may be reachable from outside", name, svc.Ports)
		}
	}
	if len(cfg.Services["proxy"].Ports) == 0 {
		t.Error("the proxy publishes nothing")
	}
}

func TestDatabaseNetworkHasNoRouteOut(t *testing.T) {
	dockertest.Require(t)
	cfg, _, err := render(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Networks["db"].Internal {
		t.Error("the database network is not internal")
	}
	if _, onWeb := cfg.Services["postgres"].Networks["web"]; onWeb {
		t.Error("the database is on the network the proxy can reach")
	}
	if _, ok := cfg.Services["proxy"].Networks["db"]; ok {
		t.Error("the proxy can reach the database")
	}
}

func TestControlPlaneRunsHardened(t *testing.T) {
	dockertest.Require(t)
	cfg, _, err := render(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	api := cfg.Services["api"]
	if !api.ReadOnly || len(api.CapDrop) == 0 || api.CapDrop[0] != "ALL" || !contains(api.SecurityOpt, "no-new-privileges:true") {
		t.Errorf("api is not hardened: %+v", api)
	}
	if api.Environment["FORGEDECK_COOKIE_SECURE"] != "true" {
		t.Error("cookies are not secure behind TLS")
	}
	if api.Healthcheck == nil || !strings.Contains(strings.Join(api.Healthcheck.Test, " "), "healthcheck") {
		t.Errorf("api health check %+v", api.Healthcheck)
	}
	if api.Environment["FORGEDECK_PUBLIC_URL"] != "https://forgedeck.example.com" {
		t.Errorf("public URL %q", api.Environment["FORGEDECK_PUBLIC_URL"])
	}
}

// The API believes X-Forwarded-For only from the proxy, so the proxy's address
// must be fixed and be exactly the trusted one.
func TestTrustedProxyIsTheProxysFixedAddress(t *testing.T) {
	dockertest.Require(t)
	cfg, _, err := render(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	fixed := cfg.Services["proxy"].Networks["web"].IPv4
	if fixed == "" || cfg.Services["api"].Environment["FORGEDECK_TRUSTED_PROXIES"] != fixed {
		t.Errorf("proxy address %q, trusted proxies %q", fixed, cfg.Services["api"].Environment["FORGEDECK_TRUSTED_PROXIES"])
	}
}

func TestProjectNameDoesNotClashWithTheDevelopmentStack(t *testing.T) {
	dockertest.Require(t)
	cfg, _, err := render(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name == "forgedeck" || cfg.Name == "" {
		t.Errorf("project name %q would share containers and volumes with the development docker-compose.yml", cfg.Name)
	}
}

func TestMissingRequiredSettingsAreRefused(t *testing.T) {
	dockertest.Require(t)
	for _, missing := range []string{"FORGEDECK_DOMAIN", "POSTGRES_PASSWORD", "FORGEDECK_SECRET_KEYS"} {
		env := map[string]string{}
		for k, v := range valid {
			if k != missing {
				env[k] = v
			}
		}
		if _, out, err := render(t, env); err == nil || !strings.Contains(out, missing) {
			t.Errorf("without %s: %v\n%s", missing, err, out)
		}
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
