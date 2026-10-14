package projects_test

import (
	"net/http"
	"net/url"
	"strings"
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

func TestGetProjectHidesNonMemberProjects(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	outsider, _ := env.NewUser("outsider@example.com")
	p := createProject(t, owner, "Secret", "")

	owner.Do(http.MethodGet, "/api/v1/projects/"+p.ID.String(), nil).RequireStatus(http.StatusOK)
	outsider.Do(http.MethodGet, "/api/v1/projects/"+p.ID.String(), nil).RequireStatus(http.StatusNotFound)
	owner.Do(http.MethodGet, "/api/v1/projects/not-a-uuid", nil).RequireStatus(http.StatusNotFound)
}

func TestUpdateProjectRequiresAdmin(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	dev, devID := env.NewUser("dev@example.com")
	p := createProject(t, owner, "Old Name", "")
	env.AddMember(p.ID, devID, "developer")
	path := "/api/v1/projects/" + p.ID.String()

	dev.Do(http.MethodPatch, path, map[string]string{"name": "Hijacked"}).RequireStatus(http.StatusForbidden)

	var updated projects.Project
	owner.Do(http.MethodPatch, path, map[string]string{"name": "New Name"}).RequireStatus(http.StatusOK).Decode(&updated)
	if updated.Name != "New Name" || updated.Slug != "old-name" {
		t.Fatalf("got %+v", updated)
	}

	createProject(t, owner, "Other", "other")
	owner.Do(http.MethodPatch, path, map[string]string{"slug": "other"}).RequireStatus(http.StatusUnprocessableEntity)
}

func TestDeleteProjectRequiresOwnerAndConfirmation(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	admin, adminID := env.NewUser("admin@example.com")
	p := createProject(t, owner, "Doomed", "doomed")
	env.AddMember(p.ID, adminID, "admin")
	path := "/api/v1/projects/" + p.ID.String()

	admin.Do(http.MethodDelete, path, map[string]string{"confirm": "doomed"}).RequireStatus(http.StatusForbidden)
	owner.Do(http.MethodDelete, path, map[string]string{"confirm": "wrong"}).RequireStatus(http.StatusUnprocessableEntity)
	owner.Do(http.MethodDelete, path, map[string]string{"confirm": "doomed"}).RequireStatus(http.StatusNoContent)
	owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusNotFound)
}

func TestListProjectsFiltersByNameOrSlug(t *testing.T) {
	env := apitest.New(t)
	dev, _ := env.NewUser("dev@example.com")
	createProject(t, dev, "Shop Backend", "shop")
	createProject(t, dev, "Billing", "money")
	createProject(t, dev, "100% Done", "done")

	names := func(query string) []string {
		var list httpx.List[projects.Project]
		dev.Do(http.MethodGet, "/api/v1/projects?q="+url.QueryEscape(query), nil).RequireStatus(http.StatusOK).Decode(&list)
		out := make([]string, len(list.Items))
		for i, p := range list.Items {
			out[i] = p.Name
		}
		return out
	}
	for query, want := range map[string]string{
		"shop":  "Shop Backend", // name, any case
		"MONEY": "Billing",      // slug
		"100%":  "100% Done",    // a wildcard is plain text
	} {
		if got := names(query); len(got) != 1 || got[0] != want {
			t.Errorf("q=%q: got %v, want [%s]", query, got, want)
		}
	}
	if got := names("%"); len(got) != 1 {
		t.Errorf("q=%%: a lone wildcard matched %v", got)
	}
	if got := names("nothing like this"); len(got) != 0 {
		t.Errorf("no match returned %v", got)
	}
	dev.Do(http.MethodGet, "/api/v1/projects?q="+strings.Repeat("a", 101), nil).RequireStatus(http.StatusBadRequest)
}
