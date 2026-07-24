// Package rbac implements role-based access control. Roles define which actions
// a user may perform. The middleware enforces this on every mutating endpoint.
package rbac

// Role names a user role.
type Role string

const (
	RoleViewer       Role = "viewer"
	RolePlanner      Role = "planner"
	RoleOperator     Role = "operator"
	RoleApprover     Role = "approver"
	RolePlatformAdmin Role = "platform_admin"
	RoleAuditor      Role = "auditor"
)

// Action is a permission verb.
type Action string

const (
	ActionView         Action = "view"
	ActionCreatePlan   Action = "create_plan"
	ActionDeletePlan   Action = "delete_plan"
	ActionRunPreflight Action = "run_preflight"
	ActionApprove      Action = "approve"
	ActionExecute      Action = "execute"
	ActionRollback     Action = "rollback"
	ActionCleanup      Action = "cleanup"
	ActionManageConn   Action = "manage_connections"
)

// User represents an authenticated operator.
type User struct {
	ID    string
	Name  string
	Email string
	Roles []Role
}

// permissions maps role -> allowed actions.
var permissions = map[Role][]Action{
	RoleViewer:        {ActionView},
	RolePlanner:       {ActionView, ActionCreatePlan, ActionDeletePlan, ActionRunPreflight},
	RoleOperator:      {ActionView, ActionCreatePlan, ActionRunPreflight, ActionExecute},
	RoleApprover:      {ActionView, ActionApprove, ActionRollback},
	RolePlatformAdmin: {ActionView, ActionCreatePlan, ActionDeletePlan, ActionRunPreflight, ActionExecute, ActionApprove, ActionRollback, ActionCleanup, ActionManageConn},
	RoleAuditor:       {ActionView},
}

// Can reports whether the user may perform the action.
func (u User) Can(action Action) bool {
	for _, role := range u.Roles {
		for _, a := range permissions[role] {
			if a == action {
				return true
			}
		}
	}
	return false
}

// MockUser returns a default admin user for mock/dev mode (no auth).
func MockUser() User {
	return User{ID: "mock-user", Name: "Mock Operator", Email: "mock@local", Roles: []Role{RolePlatformAdmin}}
}