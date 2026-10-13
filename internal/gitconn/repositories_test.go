package gitconn_test

import (
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/github"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
)

type repoList struct {
	Items []gitconn.Repository `json:"items"`
}

func TestRepositoriesFromGitHub(t *testing.T) {
	f := newGitHubFixture(t)
	f.fake.AddInstallation(11, "acme",
		github.Repository{ID: 1, FullName: "acme/api", DefaultBranch: "main", Private: true},
		github.Repository{ID: 2, FullName: "acme/web", DefaultBranch: "trunk"})
	f.callback(t, f.startInstall(t), "11", "acme-admin-code")
	conn := f.connections(t)[0]

	var list repoList
	f.owner.Do(http.MethodGet, "/api/v1/git-connections/"+conn.ID.String()+"/repositories", nil).
		RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 2 || list.Items[0].FullName != "acme/api" || !list.Items[0].Private || list.Items[1].DefaultBranch != "trunk" {
		t.Fatalf("repositories %+v", list.Items)
	}
}

func TestRepositoriesFromGitLab(t *testing.T) {
	fake := gitlabtest.New(t)
	fake.AddToken("glpat-good", gitlabtest.Token{Username: "bot", Scopes: []string{"api"}, Projects: []gitlab.Project{
		{ID: 77, PathWithNamespace: "acme/shop", DefaultBranch: "main", Visibility: "private"},
	}})
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	viewer, viewerID := env.NewUser("viewer@example.com")
	projectID := owner.CreateProject("Shop")
	env.AddMember(projectID, viewerID, "viewer")

	var conn gitconn.Connection
	owner.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/git-connections/gitlab",
		map[string]string{"base_url": fake.URL, "token": "glpat-good"}).RequireStatus(http.StatusCreated).Decode(&conn)
	path := "/api/v1/git-connections/" + conn.ID.String() + "/repositories"

	var list repoList
	owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK).Decode(&list)
	if len(list.Items) != 1 || list.Items[0].ID != "77" || list.Items[0].FullName != "acme/shop" || !list.Items[0].Private {
		t.Fatalf("repositories %+v", list.Items)
	}
	viewer.Do(http.MethodGet, path, nil).RequireStatus(http.StatusForbidden)
}
