package servers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

func TestCreateEnrollmentToken(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Infra")
	path := "/api/v1/projects/" + projectID.String() + "/servers/enrollment-tokens"

	resp := owner.Do(http.MethodPost, path, map[string]string{"server_name": "prod-01"}).RequireStatus(http.StatusCreated)
	var token servers.EnrollmentToken
	resp.Decode(&token)

	if len(token.Token) < 40 || token.Token[:4] != "fde_" || token.ServerName != "prod-01" {
		t.Fatalf("got %+v", token)
	}
	if ttl := time.Until(token.ExpiresAt); ttl < 50*time.Minute || ttl > time.Hour {
		t.Fatalf("expires in %v, want about 1h", ttl)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("token response may be cached")
	}
}

func TestCreateEnrollmentTokenRules(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	dev, devID := env.NewUser("dev@example.com")
	outsider, _ := env.NewUser("outsider@example.com")
	projectID := owner.CreateProject("Infra")
	env.AddMember(projectID, devID, "developer")
	path := "/api/v1/projects/" + projectID.String() + "/servers/enrollment-tokens"

	dev.Do(http.MethodPost, path, map[string]string{"server_name": "x"}).RequireStatus(http.StatusForbidden)
	outsider.Do(http.MethodPost, path, map[string]string{"server_name": "x"}).RequireStatus(http.StatusNotFound)
	owner.Do(http.MethodPost, path, map[string]string{"server_name": " "}).RequireStatus(http.StatusUnprocessableEntity)
}
