package registries_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/registries"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

func validRegistry() map[string]string {
	return map[string]string{"name": "GHCR", "host": "https://GHCR.io/", "username": "acme-bot", "password": "ghp_secret"}
}

func TestCreateListAndDeleteRegistry(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	base := "/api/v1/projects/" + projectID.String() + "/registries"

	resp := owner.Do(http.MethodPost, base, validRegistry()).RequireStatus(http.StatusCreated)
	if strings.Contains(string(resp.Body), "ghp_secret") {
		t.Fatal("response contains the password")
	}
	var created registries.Registry
	resp.Decode(&created)
	if created.Host != "ghcr.io" || created.Username != "acme-bot" {
		t.Fatalf("created %+v", created)
	}

	svc := registries.NewService(store.New(env.Pool), authz.New(store.New(env.Pool)), env.Config.Keyring)
	creds, err := svc.Credentials(t.Context(), created.ID)
	if err != nil || creds.Password != "ghp_secret" || creds.Host != "ghcr.io" {
		t.Fatalf("credentials %+v, %v", creds, err)
	}

	var list struct{ Items []registries.Registry }
	owner.Do(http.MethodGet, base, nil).RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 1 || list.Items[0].ID != created.ID {
		t.Fatalf("list %+v", list.Items)
	}
	owner.Do(http.MethodPost, base, validRegistry()).RequireStatus(http.StatusUnprocessableEntity)

	owner.Do(http.MethodDelete, "/api/v1/registries/"+created.ID.String(), nil).RequireStatus(http.StatusNoContent)
	owner.Do(http.MethodDelete, "/api/v1/registries/"+created.ID.String(), nil).RequireStatus(http.StatusNotFound)
}

func TestRegistryValidation(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	base := "/api/v1/projects/" + owner.CreateProject("Shop").String() + "/registries"
	cases := map[string]func(map[string]string){
		"host with path":    func(b map[string]string) { b["host"] = "ghcr.io/acme" },
		"host with spaces":  func(b map[string]string) { b["host"] = "my registry" },
		"empty password":    func(b map[string]string) { b["password"] = "" },
		"username id colon": func(b map[string]string) { b["username"] = "a:b" },
		"empty name":        func(b map[string]string) { b["name"] = " " },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			body := validRegistry()
			mutate(body)
			owner.Do(http.MethodPost, base, body).RequireStatus(http.StatusUnprocessableEntity)
		})
	}
}

func TestRegistryAuthorization(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	base := "/api/v1/projects/" + projectID.String() + "/registries"
	var created registries.Registry
	owner.Do(http.MethodPost, base, validRegistry()).RequireStatus(http.StatusCreated).Decode(&created)

	dev, devID := env.NewUser("dev@example.com")
	env.AddMember(projectID, devID, "developer")
	dev.Do(http.MethodGet, base, nil).RequireStatus(http.StatusOK)
	dev.Do(http.MethodPost, base, validRegistry()).RequireStatus(http.StatusForbidden)
	dev.Do(http.MethodDelete, "/api/v1/registries/"+created.ID.String(), nil).RequireStatus(http.StatusForbidden)

	outsider, _ := env.NewUser("outsider@example.com")
	outsider.Do(http.MethodGet, base, nil).RequireStatus(http.StatusNotFound)
	outsider.Do(http.MethodDelete, "/api/v1/registries/"+created.ID.String(), nil).RequireStatus(http.StatusNotFound)
}
