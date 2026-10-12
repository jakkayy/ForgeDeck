package gitconn_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
	"github.com/jakkayy/ForgeDeck/internal/safehttp"
)

func TestConnectGitLab(t *testing.T) {
	fake := gitlabtest.New(t)
	fake.AddToken("glpat-good", gitlabtest.Token{Username: "project_7_bot", Scopes: []string{"api"}})
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	projectID := owner.CreateProject("Shop")
	path := "/api/v1/projects/" + projectID.String() + "/git-connections/gitlab"

	var conn gitconn.Connection
	owner.Do(http.MethodPost, path, map[string]string{"base_url": fake.URL + "/", "token": "glpat-good"}).
		RequireStatus(http.StatusCreated).Decode(&conn)
	if conn.Provider != "gitlab" || conn.BaseURL != fake.URL || conn.DisplayName == "" {
		t.Fatalf("connection %+v", conn)
	}

	var tokenCiphertext, webhookCiphertext []byte
	if err := env.Pool.QueryRow(t.Context(),
		"SELECT token_ciphertext, webhook_ciphertext FROM git_connections WHERE id = $1", conn.ID).
		Scan(&tokenCiphertext, &webhookCiphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(tokenCiphertext, []byte("glpat-good")) || len(webhookCiphertext) == 0 {
		t.Fatal("token stored in plaintext or webhook secret missing")
	}
}

func TestConnectGitLabValidation(t *testing.T) {
	fake := gitlabtest.New(t)
	fake.AddToken("glpat-readonly", gitlabtest.Token{Username: "bot", Scopes: []string{"read_api"}})
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	path := "/api/v1/projects/" + owner.CreateProject("Shop").String() + "/git-connections/gitlab"

	cases := map[string]map[string]string{
		"rejected token":     {"base_url": fake.URL, "token": "glpat-wrong"},
		"missing api scope":  {"base_url": fake.URL, "token": "glpat-readonly"},
		"empty token":        {"base_url": fake.URL, "token": " "},
		"not a URL":          {"base_url": "gitlab dot com", "token": "x"},
		"credentials in URL": {"base_url": "https://user:pass@gitlab.example.com", "token": "x"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			resp := owner.Do(http.MethodPost, path, body).RequireStatus(http.StatusUnprocessableEntity)
			if resp.ErrorCode() != "validation_failed" {
				t.Fatalf("code %s", resp.ErrorCode())
			}
		})
	}
}

func TestConnectGitLabBlocksInternalAddressesByDefault(t *testing.T) {
	fake := gitlabtest.New(t)
	fake.AddToken("glpat-good", gitlabtest.Token{Username: "bot", Scopes: []string{"api"}})
	env := apitest.New(t, func(cfg *config.Config) { cfg.Outbound = safehttp.Policy{} })
	owner, _ := env.NewUser("owner@example.com")
	path := "/api/v1/projects/" + owner.CreateProject("Shop").String() + "/git-connections/gitlab"

	// Production policy: no plain http, and no loopback even over https.
	owner.Do(http.MethodPost, path, map[string]string{"base_url": fake.URL, "token": "glpat-good"}).
		RequireStatus(http.StatusUnprocessableEntity)
	owner.Do(http.MethodPost, path, map[string]string{"base_url": "https://127.0.0.1:1", "token": "glpat-good"}).
		RequireStatus(http.StatusUnprocessableEntity)
}
