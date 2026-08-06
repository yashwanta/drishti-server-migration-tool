package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

func serviceForRole(t *testing.T, role rbac.Role) (*auth.Service, string, string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.New([]auth.UserCredential{{User: rbac.User{ID: string(role), Roles: []rbac.Role{role}}, Username: string(role), PasswordHash: string(hash)}}, time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	token, csrf, _, err := service.Login(string(role), "password")
	if err != nil {
		t.Fatal(err)
	}
	return service, token, csrf
}

func concretePath(pattern string) string {
	path := strings.TrimPrefix(pattern, "GET ")
	path = strings.TrimPrefix(path, "POST ")
	path = strings.TrimPrefix(path, "DELETE ")
	return strings.ReplaceAll(path, "{id}", "test-id")
}

func methodOf(pattern string) string { return pattern[:strings.IndexByte(pattern, ' ')] }

func TestCompleteRoutePermissionMatrix(t *testing.T) {
	roles := []rbac.Role{rbac.RoleViewer, rbac.RolePlanner, rbac.RoleOperator, rbac.RoleApprover, rbac.RolePlatformAdmin, rbac.RoleAuditor}
	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			service, token, csrf := serviceForRole(t, role)
			srv := New(mustCfg(), nil, service)
			for _, route := range protectedPatterns() {
				route := route
				srv.Router().HandleFunc(route.pattern, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
			}
			user := rbac.User{Roles: []rbac.Role{role}}
			for _, route := range protectedPatterns() {
				req := httptest.NewRequest(methodOf(route.pattern), concretePath(route.pattern), nil)
				req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
				req.Header.Set("X-CSRF-Token", csrf)
				w := httptest.NewRecorder()
				srv.Handler().ServeHTTP(w, req)
				want := http.StatusForbidden
				if user.Can(route.action) {
					want = http.StatusNoContent
				}
				if w.Code != want {
					t.Errorf("%s: status=%d want=%d action=%s", route.pattern, w.Code, want, route.action)
				}
			}
		})
	}
}

func TestProtectedRoutesRequireAuthentication(t *testing.T) {
	service, _, _ := serviceForRole(t, rbac.RolePlatformAdmin)
	srv := New(mustCfg(), nil, service)
	for _, route := range protectedPatterns() {
		req := httptest.NewRequest(methodOf(route.pattern), concretePath(route.pattern), nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: status=%d want=401", route.pattern, w.Code)
		}
	}
}
