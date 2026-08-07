package server

import (
	"net/http"

	"github.com/drishti/hypershift/internal/rbac"
)

type protectedPattern struct {
	pattern string
	action  rbac.Action
}

// protectedPatterns is the complete, fail-closed API permission matrix.
// Adding a handler to the inner router does not make it externally reachable;
// it must also be listed here with an explicit action.
func protectedPatterns() []protectedPattern {
	return []protectedPattern{
		{"GET /api/v1/connections", rbac.ActionView},
		{"POST /api/v1/connections", rbac.ActionManageConn},
		{"POST /api/v1/connections/probe", rbac.ActionManageConn},
		{"GET /api/v1/connections/{id}", rbac.ActionView},
		{"DELETE /api/v1/connections/{id}", rbac.ActionManageConn},
		{"GET /api/v1/connections/{id}/inventory", rbac.ActionView},
		{"POST /api/v1/connections/{id}/test", rbac.ActionManageConn},
		{"GET /api/v1/runtime", rbac.ActionView},
		{"GET /api/v1/plans", rbac.ActionView},
		{"POST /api/v1/plans", rbac.ActionCreatePlan},
		{"GET /api/v1/plans/{id}", rbac.ActionView},
		{"DELETE /api/v1/plans/{id}", rbac.ActionDeletePlan},
		{"GET /api/v1/audit", rbac.ActionViewAudit},
		{"GET /api/v1/users", rbac.ActionManageUsers},
		{"POST /api/v1/users", rbac.ActionManageUsers},
		{"POST /api/v1/users/{id}/deactivate", rbac.ActionManageUsers},
		{"POST /api/v1/plans/{id}/preflight", rbac.ActionRunPreflight},
		{"POST /api/v1/plans/{id}/approve", rbac.ActionApprove},
		{"POST /api/v1/plans/{id}/execute", rbac.ActionExecute},
		{"GET /api/v1/jobs", rbac.ActionView},
		{"GET /api/v1/jobs/{id}", rbac.ActionView},
		{"POST /api/v1/jobs/{id}/validate", rbac.ActionExecute},
		{"POST /api/v1/jobs/{id}/cutover", rbac.ActionExecute},
		{"POST /api/v1/jobs/{id}/rollback", rbac.ActionRollback},
		{"GET /api/v1/jobs/{id}/report", rbac.ActionView},
	}
}

func (s *Server) protectAPI() {
	for _, route := range protectedPatterns() {
		s.root.Handle(route.pattern, s.auth.Require(route.action, s.router))
	}
}

var _ http.Handler = (*http.ServeMux)(nil)
