package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/drishti/hypershift/internal/api"
	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
	"github.com/drishti/hypershift/internal/rbac"
	"github.com/drishti/hypershift/internal/server"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthenticatedHandlersAndExecutionLockIntegration(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.New([]auth.UserCredential{{
		User:     rbac.User{ID: "admin-1", Name: "Admin", Roles: []rbac.Role{rbac.RolePlatformAdmin}},
		Username: "admin", PasswordHash: string(hash),
	}}, time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(config.Config{Mode: config.ModeMock}, nil, authService)
	handlers := api.NewHandlers(mock.New(), api.NewPlanStore(), api.NewAuditStore())
	handlers.Register(srv.Router())
	api.NewMigrationHandlers(handlers, nil, api.NewMockFactory(), t.TempDir()).RegisterMigration(srv.Router())

	unauth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/api/v1/connections", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", unauth.Code)
	}

	token, csrf, _, err := authService.Login("admin", "password")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}

	if w := request(http.MethodGet, "/api/v1/connections", ""); w.Code != http.StatusOK {
		t.Fatalf("authorized read status=%d body=%s", w.Code, w.Body.String())
	}
	create := request(http.MethodPost, "/api/v1/plans", `{"source_vm_id":"vm-1","source_connection_id":"conn-vmware-1","target_node_id":"pve-01","target_connection_id":"conn-proxmox-1"}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("authorized create status=%d body=%s", create.Code, create.Body.String())
	}
	var plan domain.Plan
	if err := json.Unmarshal(create.Body.Bytes(), &plan); err != nil || plan.CreatedBy != "admin-1" {
		t.Fatalf("audit identity not propagated: plan=%#v err=%v", plan, err)
	}
	for _, path := range []string{"/api/v1/plans/any/execute", "/api/v1/jobs/any/cutover", "/api/v1/jobs/any/rollback"} {
		if w := request(http.MethodPost, path, ""); w.Code != http.StatusNotImplemented || !strings.Contains(w.Body.String(), "execution_disabled") {
			t.Fatalf("%s escaped execution lock: status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}
