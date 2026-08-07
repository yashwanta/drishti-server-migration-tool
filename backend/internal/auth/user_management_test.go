package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

func userManagementService(t *testing.T) (*Service, *MemoryUserStore, *[]domain.AuditEvent) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("admin-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	events := []domain.AuditEvent{}
	var auditMu sync.Mutex
	store, err := NewMemoryUserStore([]UserRecord{
		{ID: "admin-1", Username: "admin-one", Name: "Admin One", Roles: []rbac.Role{rbac.RolePlatformAdmin}, Active: true, PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now},
		{ID: "admin-2", Username: "admin-two", Name: "Admin Two", Roles: []rbac.Role{rbac.RolePlatformAdmin}, Active: true, PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now},
	}, func(_ context.Context, event domain.AuditEvent) error {
		auditMu.Lock()
		defer auditMu.Unlock()
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewWithStore(store, time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	return service, store, &events
}

func authenticatedRequest(t *testing.T, service *Service, method, path, body, username, password string, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	token, csrf, _, err := service.Login(username, password)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if strings.HasSuffix(path, "/deactivate") {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) >= 2 {
			req.SetPathValue("id", parts[len(parts)-2])
		}
	}
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	req.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestCreateListDeactivateAndAuditUsers(t *testing.T) {
	service, _, events := userManagementService(t)
	create := service.Require(rbac.ActionManageUsers, http.HandlerFunc(service.CreateUserHandler))
	w := authenticatedRequest(t, service, http.MethodPost, "/api/v1/users", `{"username":"new.operator","roles":["operator","viewer"],"password":"new-user-password"}`, "admin-one", "admin-password", create)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("password")) || bytes.Contains(w.Body.Bytes(), []byte("hash")) {
		t.Fatalf("secret material in response: %s", w.Body.String())
	}
	var created ManagedUser
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Username != "new.operator" || !created.Active {
		t.Fatalf("created user=%#v", created)
	}
	if len(*events) != 2 || (*events)[0].Actor != "admin-1" || (*events)[0].Action != "user.create" || (*events)[1].Action != "user.roles.assign" {
		t.Fatalf("audit events=%#v", *events)
	}
	for _, event := range *events {
		if strings.Contains(event.Detail, "new-user-password") || strings.Contains(event.Detail, "$2") {
			t.Fatalf("secret material in audit: %#v", event)
		}
	}
	newToken, _, _, err := service.Login("new.operator", "new-user-password")
	if err != nil {
		t.Fatal(err)
	}
	deactivate := service.Require(rbac.ActionManageUsers, http.HandlerFunc(service.DeactivateUserHandler))
	w = authenticatedRequest(t, service, http.MethodPost, "/api/v1/users/"+created.ID+"/deactivate", "", "admin-one", "admin-password", deactivate)
	if w.Code != http.StatusOK {
		t.Fatalf("deactivate status=%d body=%s", w.Code, w.Body.String())
	}
	if _, ok := service.lookup(newToken); ok {
		t.Fatal("deactivated user's session remains active")
	}
	if _, _, _, err := service.Login("new.operator", "new-user-password"); err != ErrInvalidCredentials {
		t.Fatalf("disabled login error=%v", err)
	}
}

func TestPasswordChangeRotatesEverySession(t *testing.T) {
	service, _, events := userManagementService(t)
	tokenOne, csrf, _, err := service.Login("admin-one", "admin-password")
	if err != nil {
		t.Fatal(err)
	}
	tokenTwo, _, _, err := service.Login("admin-one", "admin-password")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`{"current_password":"admin-password","new_password":"replacement-password"}`))
	req.AddCookie(&http.Cookie{Name: CookieName, Value: tokenOne})
	req.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	service.RequireSession(http.HandlerFunc(service.ChangePasswordHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("change status=%d body=%s", w.Code, w.Body.String())
	}
	if _, ok := service.lookup(tokenOne); ok {
		t.Fatal("current session not rotated")
	}
	if _, ok := service.lookup(tokenTwo); ok {
		t.Fatal("second session not rotated")
	}
	if _, _, _, err := service.Login("admin-one", "admin-password"); err != ErrInvalidCredentials {
		t.Fatalf("old password error=%v", err)
	}
	if _, _, _, err := service.Login("admin-one", "replacement-password"); err != nil {
		t.Fatalf("new password login: %v", err)
	}
	if len(*events) != 1 || (*events)[0].Action != "user.password_change" || strings.Contains((*events)[0].Detail, "replacement-password") {
		t.Fatalf("password audit=%#v", *events)
	}
}

func TestAdminDeactivationProtections(t *testing.T) {
	service, store, _ := userManagementService(t)
	selfEvent := domain.AuditEvent{ID: "evt-self", Timestamp: time.Now().UTC(), Actor: "admin-1", Action: "user.deactivate", Target: "admin-1"}
	if _, err := store.DeactivateUser(context.Background(), "admin-1", "admin-1", selfEvent); err != ErrSelfDeactivate {
		t.Fatalf("self deactivation error=%v", err)
	}
	secondEvent := domain.AuditEvent{ID: "evt-second", Timestamp: time.Now().UTC(), Actor: "admin-1", Action: "user.deactivate", Target: "admin-2"}
	if _, err := store.DeactivateUser(context.Background(), "admin-2", "admin-1", secondEvent); err != nil {
		t.Fatal(err)
	}
	lastEvent := domain.AuditEvent{ID: "evt-last", Timestamp: time.Now().UTC(), Actor: "external-admin", Action: "user.deactivate", Target: "admin-1"}
	if _, err := store.DeactivateUser(context.Background(), "admin-1", "external-admin", lastEvent); err != ErrLastPlatformAdmin {
		t.Fatalf("last admin deactivation error=%v", err)
	}
	_ = service
}

func TestWrongCurrentPasswordIsGeneric(t *testing.T) {
	service, _, _ := userManagementService(t)
	w := authenticatedRequest(t, service, http.MethodPost, "/api/v1/auth/change-password", `{"current_password":"incorrect-value","new_password":"replacement-password"}`, "admin-one", "admin-password", service.RequireSession(http.HandlerFunc(service.ChangePasswordHandler)))
	if w.Code != http.StatusUnauthorized || strings.TrimSpace(w.Body.String()) != `{"error":"invalid credentials"}` {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}
