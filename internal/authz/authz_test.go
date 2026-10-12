package authz

import "testing"

func TestRoleCan(t *testing.T) {
	want := map[Role][]Permission{
		RoleViewer:    {ViewProject},
		RoleDeveloper: {ViewProject, OperateContainer},
		RoleAdmin:     {ViewProject, OperateContainer, UpdateProject, ManageMembers, ManageServers, ManageGit, ManageApps},
		RoleOwner:     {ViewProject, OperateContainer, UpdateProject, ManageMembers, ManageServers, ManageGit, ManageApps, DeleteProject},
	}
	all := []Permission{ViewProject, OperateContainer, UpdateProject, ManageMembers, ManageServers, ManageGit, ManageApps, DeleteProject}

	for role, granted := range want {
		allowed := map[Permission]bool{}
		for _, p := range granted {
			allowed[p] = true
		}
		for _, p := range all {
			if got := role.Can(p); got != allowed[p] {
				t.Errorf("%s.Can(%s) = %v, want %v", role, p, got, allowed[p])
			}
		}
	}
}

func TestUnknownRoleAndPermissionDenied(t *testing.T) {
	if Role("superuser").Can(ViewProject) {
		t.Error("unknown role granted a permission")
	}
	if RoleOwner.Can(Permission("unknown")) {
		t.Error("unknown permission granted")
	}
}

func TestParseRole(t *testing.T) {
	if r, ok := ParseRole("developer"); !ok || r != RoleDeveloper {
		t.Errorf("ParseRole(developer) = %q, %v", r, ok)
	}
	if _, ok := ParseRole("root"); ok {
		t.Error("ParseRole accepted root")
	}
}
