package compose_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agent/compose"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
)

const imageRef = "ghcr.io/acme/shop-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func ptr[T any](v T) *T { return &v }

func spec(config servicecfg.Config) compose.Spec {
	return compose.Spec{EnvironmentID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), ImageRef: imageRef, Config: config}
}

func render(t *testing.T, s compose.Spec) map[string]any {
	t.Helper()
	data, err := compose.Render(s)
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, data)
	}
	return file
}

func service(file map[string]any) map[string]any {
	return file["services"].(map[string]any)[compose.ServiceName].(map[string]any)
}

func TestRenderFullConfiguration(t *testing.T) {
	file := render(t, spec(servicecfg.Config{
		ContainerPort: ptr(8080), PublishedPort: ptr(18080), BindAddress: servicecfg.BindAll,
		Env:     map[string]string{"LOG_LEVEL": "info"},
		Volumes: []servicecfg.Volume{{Name: "app-data", Path: "/var/lib/app"}},
		Restart: servicecfg.RestartAlways, MemoryMB: ptr(512), CPUs: ptr(1.5),
	}))

	if file["name"] != "forgedeck-11111111-1111-1111-1111-111111111111" {
		t.Fatalf("project %v", file["name"])
	}
	svc := service(file)
	if svc["image"] != imageRef || svc["restart"] != "always" || svc["mem_limit"] != "512m" || svc["cpus"] != 1.5 {
		t.Fatalf("service %+v", svc)
	}
	if ports := svc["ports"].([]any); len(ports) != 1 || ports[0] != "0.0.0.0:18080:8080" {
		t.Fatalf("ports %v", svc["ports"])
	}
	if vols := svc["volumes"].([]any); len(vols) != 1 || vols[0] != "app-data:/var/lib/app" {
		t.Fatalf("volumes %v", svc["volumes"])
	}
	if _, ok := file["volumes"].(map[string]any)["app-data"]; !ok {
		t.Fatalf("top-level volumes %v", file["volumes"])
	}
	labels := svc["labels"].(map[string]any)
	if labels[compose.LabelManaged] != "true" || labels[compose.LabelEnvironment] != "11111111-1111-1111-1111-111111111111" || len(labels) != 2 {
		t.Fatalf("labels %v", labels)
	}
}

func TestRenderDefaultsAreSafe(t *testing.T) {
	file := render(t, spec(servicecfg.Config{ContainerPort: ptr(8080), PublishedPort: ptr(18080)}))
	svc := service(file)
	if ports := svc["ports"].([]any); ports[0] != "127.0.0.1:18080:8080" {
		t.Fatalf("ports %v", ports)
	}
	if svc["restart"] != "unless-stopped" {
		t.Fatalf("restart %v", svc["restart"])
	}
	for _, key := range []string{"privileged", "network_mode", "cap_add", "devices", "pid", "ipc", "command", "entrypoint", "user"} {
		if _, ok := svc[key]; ok {
			t.Errorf("unexpected key %q", key)
		}
	}
	if _, ok := file["volumes"]; ok {
		t.Error("top-level volumes without any volume")
	}
	// Without a published port, nothing is exposed.
	if _, ok := service(render(t, spec(servicecfg.Config{ContainerPort: ptr(8080)})))["ports"]; ok {
		t.Error("ports without published_port")
	}
}

func TestRenderTreatsUserTextAsData(t *testing.T) {
	nasty := "x\"\n    privileged: true\n$(rm -rf /) ${HOME} $$ #comment: {a: b}"
	file := render(t, spec(servicecfg.Config{Env: map[string]string{"NASTY": nasty}}))
	svc := service(file)

	env := svc["environment"].(map[string]any)
	if env["NASTY"] != strings.ReplaceAll(nasty, "$", "$$") {
		t.Fatalf("env %q", env["NASTY"])
	}
	if _, ok := svc["privileged"]; ok {
		t.Fatal("env value became a service key")
	}
}

func TestRenderReferencesSecretsWithoutTheirValues(t *testing.T) {
	file := render(t, spec(servicecfg.Config{
		Env: map[string]string{"LOG_LEVEL": "info"}, Secrets: []string{"DATABASE_PASSWORD"},
	}))
	env := service(file)["environment"].(map[string]any)
	if env["DATABASE_PASSWORD"] != "${DATABASE_PASSWORD}" || env["LOG_LEVEL"] != "info" {
		t.Fatalf("environment %v", env)
	}
}

func TestRenderRefusesTamperedInput(t *testing.T) {
	cases := map[string]compose.Spec{
		"mutable tag":      {ImageRef: "ghcr.io/acme/app:latest", Config: servicecfg.Config{}},
		"image with space": {ImageRef: "ghcr.io/acme/app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef --privileged", Config: servicecfg.Config{}},
		"host mount":       {ImageRef: imageRef, Config: servicecfg.Config{Volumes: []servicecfg.Volume{{Name: "/var/run/docker.sock", Path: "/sock"}}}},
		"low host port":    {ImageRef: imageRef, Config: servicecfg.Config{ContainerPort: ptr(80), PublishedPort: ptr(22)}},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := compose.Render(s); err == nil {
				t.Fatal("rendered")
			}
		})
	}
}

// TestComposeAcceptsRenderedFile asks the real Compose plugin to parse the
// output and keeps "$" in values literal.
func TestComposeAcceptsRenderedFile(t *testing.T) {
	if os.Getenv("FORGEDECK_TEST_DOCKER") != "1" {
		t.Skip("set FORGEDECK_TEST_DOCKER=1 to run against Docker Compose")
	}
	data, err := compose.Render(spec(servicecfg.Config{
		ContainerPort: ptr(8080), PublishedPort: ptr(18080),
		Env:     map[string]string{"PASSWORD": "pa$word ${X}"},
		Secrets: []string{"API_TOKEN"},
		Volumes: []servicecfg.Volume{{Name: "data", Path: "/var/lib/app"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "compose.yml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), "docker", "compose", "-f", path, "config", "--format", "json") //nolint:gosec // test
	cmd.Env = append(os.Environ(), "API_TOKEN=tok-from-the-environment")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config: %v\n%s", err, out)
	}
	var parsed struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
		} `json:"services"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// `config` prints values re-escaped so the output can be read back, so
	// the literal "pa$word ${X}" appears as "pa$$word $${X}". Had the value
	// been interpolated instead, "$word" and "${X}" would be empty here.
	environment := parsed.Services[compose.ServiceName].Environment
	if got := environment["PASSWORD"]; got != "pa$$word $${X}" {
		t.Fatalf("Compose resolved PASSWORD to %q", got)
	}
	if got := environment["API_TOKEN"]; got != "tok-from-the-environment" {
		t.Fatalf("Compose resolved API_TOKEN to %q, want the value from the process environment", got)
	}
	if strings.Contains(string(data), "tok-from-the-environment") {
		t.Fatal("the secret value is in the rendered file")
	}
}
