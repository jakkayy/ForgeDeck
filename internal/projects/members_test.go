package projects_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/projects"
)

type team struct {
	env      *apitest.Env
	project  projects.Project
	owner    *apitest.Client
	ownerID  uuid.UUID
	admin    *apitest.Client
	adminID  uuid.UUID
	dev      *apitest.Client
	devID    uuid.UUID
	membersP string
}

func newTeam(t *testing.T) team {
	t.Helper()
	env := apitest.New(t)
	owner, ownerID := env.NewUser("owner@example.com")
	admin, adminID := env.NewUser("admin@example.com")
	dev, devID := env.NewUser("dev@example.com")
	p := createProject(t, owner, "Team", "")
	env.AddMember(p.ID, adminID, "admin")
	env.AddMember(p.ID, devID, "developer")
	return team{
		env: env, project: p,
		owner: owner, ownerID: ownerID,
		admin: admin, adminID: adminID,
		dev: dev, devID: devID,
		membersP: "/api/v1/projects/" + p.ID.String() + "/members",
	}
}

func (tm team) memberPath(id uuid.UUID) string { return tm.membersP + "/" + id.String() }

func TestListMembers(t *testing.T) {
	tm := newTeam(t)

	var list httpx.List[projects.Member]
	tm.dev.Do(http.MethodGet, tm.membersP, nil).RequireStatus(http.StatusOK).Decode(&list)

	got := map[string]string{}
	for _, m := range list.Items {
		got[m.Email] = string(m.Role)
	}
	want := map[string]string{"owner@example.com": "owner", "admin@example.com": "admin", "dev@example.com": "developer"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for email, role := range want {
		if got[email] != role {
			t.Errorf("%s: role %q, want %q", email, got[email], role)
		}
	}
}

func TestAddMember(t *testing.T) {
	tm := newTeam(t)
	tm.env.CreateUser("new@example.com")

	var m projects.Member
	tm.admin.Do(http.MethodPost, tm.membersP, map[string]string{"email": "new@example.com", "role": "viewer"}).
		RequireStatus(http.StatusCreated).Decode(&m)
	if m.Email != "new@example.com" || m.Role != "viewer" || m.AddedAt.IsZero() {
		t.Fatalf("got %+v", m)
	}

	tm.admin.Do(http.MethodPost, tm.membersP, map[string]string{"email": "new@example.com", "role": "viewer"}).
		RequireStatus(http.StatusConflict)
}

func TestAddMemberRules(t *testing.T) {
	tm := newTeam(t)
	tm.env.CreateUser("new@example.com")

	cases := []struct {
		name   string
		actor  *apitest.Client
		body   map[string]string
		status int
	}{
		{"developer cannot manage members", tm.dev, map[string]string{"email": "new@example.com", "role": "viewer"}, http.StatusForbidden},
		{"admin cannot grant owner", tm.admin, map[string]string{"email": "new@example.com", "role": "owner"}, http.StatusForbidden},
		{"unknown role", tm.admin, map[string]string{"email": "new@example.com", "role": "root"}, http.StatusUnprocessableEntity},
		{"unknown user", tm.admin, map[string]string{"email": "ghost@example.com", "role": "viewer"}, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.actor.Do(http.MethodPost, tm.membersP, tc.body).RequireStatus(tc.status)
		})
	}
}

func TestRemoveMemberRules(t *testing.T) {
	tm := newTeam(t)

	tm.dev.Do(http.MethodDelete, tm.memberPath(tm.adminID), nil).RequireStatus(http.StatusForbidden)
	tm.admin.Do(http.MethodDelete, tm.memberPath(tm.ownerID), nil).RequireStatus(http.StatusForbidden)
	tm.owner.Do(http.MethodDelete, tm.memberPath(tm.ownerID), nil).RequireStatus(http.StatusConflict)
	tm.owner.Do(http.MethodDelete, tm.memberPath(uuid.New()), nil).RequireStatus(http.StatusNotFound)

	tm.admin.Do(http.MethodDelete, tm.memberPath(tm.devID), nil).RequireStatus(http.StatusNoContent)
	tm.dev.Do(http.MethodGet, "/api/v1/projects/"+tm.project.ID.String(), nil).RequireStatus(http.StatusNotFound)
}

func TestMemberCanLeaveProject(t *testing.T) {
	tm := newTeam(t)
	tm.dev.Do(http.MethodDelete, tm.memberPath(tm.devID), nil).RequireStatus(http.StatusNoContent)
}
