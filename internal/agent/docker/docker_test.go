package docker

import (
	"maps"
	"testing"
)

func TestContainerName(t *testing.T) {
	if got := containerName([]string{"/api", "/alias"}); got != "api" {
		t.Errorf("got %q", got)
	}
	if got := containerName(nil); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestForwardedLabelsDropsUnknownLabels(t *testing.T) {
	got := forwardedLabels(map[string]string{
		"forgedeck.application_id":               "app-1",
		"com.docker.compose.project":             "shop",
		"com.docker.compose.service":             "api",
		"com.docker.compose.config-hash":         "abc",
		"com.docker.compose.project.working_dir": "/home/dev/shop",
		"vendor.db_password":                     "hunter2",
	})
	want := map[string]string{
		"forgedeck.application_id":   "app-1",
		"com.docker.compose.project": "shop",
		"com.docker.compose.service": "api",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if forwardedLabels(map[string]string{"other": "x"}) != nil {
		t.Fatal("expected nil for no forwarded labels")
	}
}
