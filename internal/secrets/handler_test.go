package secrets_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/secrets"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

const value = "s3cret-database-password"

func put(client *apitest.Client, projectID uuid.UUID, name, v string) *apitest.Response {
	return client.Do(http.MethodPut, "/api/v1/projects/"+projectID.String()+"/secrets/"+name, map[string]string{"value": v})
}

func TestSecretsAreWriteOnlyAndVersioned(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	base := "/api/v1/projects/" + projectID.String() + "/secrets"

	resp := put(owner, projectID, "DATABASE_PASSWORD", value).RequireStatus(http.StatusCreated)
	if strings.Contains(string(resp.Body), value) {
		t.Fatalf("create response contains the value: %s", resp.Body)
	}
	var created secrets.Secret
	resp.Decode(&created)
	if created.Name != "DATABASE_PASSWORD" || created.Version != 1 {
		t.Fatalf("created %+v", created)
	}

	var replaced secrets.Secret
	put(owner, projectID, "DATABASE_PASSWORD", "another-secret-value").RequireStatus(http.StatusOK).Decode(&replaced)
	if replaced.Version != 2 {
		t.Fatalf("replaced %+v", replaced)
	}

	list := owner.Do(http.MethodGet, base, nil).RequireStatus(http.StatusOK)
	if strings.Contains(string(list.Body), "secret-value") || strings.Contains(string(list.Body), value) {
		t.Fatalf("list contains a value: %s", list.Body)
	}
	var page struct{ Items []secrets.Secret }
	list.Decode(&page)
	if len(page.Items) != 1 || page.Items[0].Name != "DATABASE_PASSWORD" || page.Items[0].Version != 2 {
		t.Fatalf("list %+v", page.Items)
	}

	// At rest the value is ciphertext, and it decrypts only for the Control Plane's own use.
	var stored []byte
	if err := env.Pool.QueryRow(context.Background(), `SELECT ciphertext FROM project_secrets`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "another-secret-value") {
		t.Fatal("secret is stored in plaintext")
	}
	svc := secrets.NewService(store.New(env.Pool), authz.New(store.New(env.Pool)), env.Config.Keyring)
	values, missing, err := svc.Values(t.Context(), projectID, []string{"DATABASE_PASSWORD", "NOT_THERE"})
	if err != nil || values["DATABASE_PASSWORD"] != "another-secret-value" || len(missing) != 1 || missing[0] != "NOT_THERE" {
		t.Fatalf("Values = %v, %v, %v", values, missing, err)
	}

	owner.Do(http.MethodDelete, base+"/DATABASE_PASSWORD", nil).RequireStatus(http.StatusNoContent)
	owner.Do(http.MethodDelete, base+"/DATABASE_PASSWORD", nil).RequireStatus(http.StatusNotFound)
}

func TestSecretValidation(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")

	cases := map[string]struct{ name, value string }{
		"lowercase name":    {"database_password", value},
		"name with a dash":  {"DB-PASSWORD", value},
		"name starts digit": {"1PASSWORD", value},
		"value too short":   {"TOKEN", "short"},
		"value with a NUL":  {"TOKEN", "abc\x00defgh"},
		"value too long":    {"TOKEN", strings.Repeat("x", 4097)},
		"name too long":     {strings.Repeat("A", 65), value},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if resp := put(owner, projectID, tc.name, tc.value); resp.Status != http.StatusUnprocessableEntity && resp.Status != http.StatusNotFound {
				t.Fatalf("status %d: %s", resp.Status, resp.Body)
			}
		})
	}
}

func TestSecretAuthorization(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	put(owner, projectID, "API_KEY", value).RequireStatus(http.StatusCreated)
	base := "/api/v1/projects/" + projectID.String() + "/secrets"

	developer, developerID := env.NewUser("dev@example.com")
	env.AddMember(projectID, developerID, "developer")
	viewer, viewerID := env.NewUser("viewer@example.com")
	env.AddMember(projectID, viewerID, "viewer")
	outsider, _ := env.NewUser("outsider@example.com")

	// Members can see names; only admins change secrets.
	viewer.Do(http.MethodGet, base, nil).RequireStatus(http.StatusOK)
	put(developer, projectID, "API_KEY", "replaced-by-developer").RequireStatus(http.StatusForbidden)
	put(viewer, projectID, "OTHER", value).RequireStatus(http.StatusForbidden)
	developer.Do(http.MethodDelete, base+"/API_KEY", nil).RequireStatus(http.StatusForbidden)

	outsider.Do(http.MethodGet, base, nil).RequireStatus(http.StatusNotFound)
	put(outsider, projectID, "OTHER", value).RequireStatus(http.StatusNotFound)
	outsider.Do(http.MethodDelete, base+"/API_KEY", nil).RequireStatus(http.StatusNotFound)

	// A secret of one project is not reachable through another's URL.
	other := owner.CreateProject("Other")
	owner.Do(http.MethodDelete, "/api/v1/projects/"+other.String()+"/secrets/API_KEY", nil).RequireStatus(http.StatusNotFound)
}
