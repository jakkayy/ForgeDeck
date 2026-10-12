package projects_test

import (
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/projects"
)

func createProject(t *testing.T, c *apitest.Client, name, slug string) projects.Project {
	t.Helper()
	var p projects.Project
	c.Do(http.MethodPost, "/api/v1/projects", map[string]string{"name": name, "slug": slug}).
		RequireStatus(http.StatusCreated).Decode(&p)
	return p
}

func TestCreateProjectMakesCallerOwner(t *testing.T) {
	env := apitest.New(t)
	dev, _ := env.NewUser("dev@example.com")

	p := createProject(t, dev, "My Startup API", "")
	if p.Slug != "my-startup-api" || p.Role != "owner" || p.Name != "My Startup API" {
		t.Fatalf("got %+v", p)
	}
}

func TestCreateProjectValidation(t *testing.T) {
	env := apitest.New(t)
	dev, _ := env.NewUser("dev@example.com")
	createProject(t, dev, "Taken", "taken")

	cases := map[string]map[string]string{
		"empty name":  {"name": "  "},
		"bad slug":    {"name": "x", "slug": "Bad Slug"},
		"slug in use": {"name": "x", "slug": "taken"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			resp := dev.Do(http.MethodPost, "/api/v1/projects", body).RequireStatus(http.StatusUnprocessableEntity)
			if resp.ErrorCode() != "validation_failed" {
				t.Fatalf("code %s", resp.ErrorCode())
			}
		})
	}
}

func TestCreateProjectWithNonLatinNameGetsGeneratedSlug(t *testing.T) {
	env := apitest.New(t)
	dev, _ := env.NewUser("dev@example.com")

	p := createProject(t, dev, "ระบบหลังบ้าน", "")
	if len(p.Slug) != len("project-")+6 {
		t.Fatalf("slug %q", p.Slug)
	}
}

func TestListProjectsShowsOnlyMemberships(t *testing.T) {
	env := apitest.New(t)
	alice, _ := env.NewUser("alice@example.com")
	bob, _ := env.NewUser("bob@example.com")
	createProject(t, alice, "Beta", "")
	createProject(t, alice, "Alpha", "")
	createProject(t, bob, "Bob Only", "")

	var list httpx.List[projects.Project]
	alice.Do(http.MethodGet, "/api/v1/projects", nil).RequireStatus(http.StatusOK).Decode(&list)

	if len(list.Items) != 2 || list.Items[0].Name != "Alpha" || list.Items[1].Name != "Beta" {
		t.Fatalf("got %+v", list.Items)
	}
	if list.NextCursor != nil {
		t.Fatalf("unexpected next cursor %q", *list.NextCursor)
	}
}

func TestListProjectsPaginates(t *testing.T) {
	env := apitest.New(t)
	dev, _ := env.NewUser("dev@example.com")
	for _, name := range []string{"a", "b", "c"} {
		createProject(t, dev, name, "")
	}

	var first, second httpx.List[projects.Project]
	dev.Do(http.MethodGet, "/api/v1/projects?limit=2", nil).RequireStatus(http.StatusOK).Decode(&first)
	if len(first.Items) != 2 || first.NextCursor == nil {
		t.Fatalf("first page %+v", first)
	}
	dev.Do(http.MethodGet, "/api/v1/projects?limit=2&cursor="+*first.NextCursor, nil).RequireStatus(http.StatusOK).Decode(&second)
	if len(second.Items) != 1 || second.Items[0].Name != "c" {
		t.Fatalf("second page %+v", second)
	}
}

func TestProjectsRequireSignIn(t *testing.T) {
	env := apitest.New(t)
	env.Anonymous().Do(http.MethodGet, "/api/v1/projects", nil).RequireStatus(http.StatusUnauthorized)
}
