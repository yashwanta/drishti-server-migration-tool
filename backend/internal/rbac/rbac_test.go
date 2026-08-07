package rbac

import "testing"

func TestManageUsersGrantedOnlyToPlatformAdmin(t *testing.T) {
	roles := []Role{RoleViewer, RolePlanner, RoleOperator, RoleApprover, RolePlatformAdmin, RoleAuditor}
	for _, role := range roles {
		got := (User{Roles: []Role{role}}).Can(ActionManageUsers)
		if got != (role == RolePlatformAdmin) {
			t.Errorf("role %s manage_users=%v", role, got)
		}
	}
}
