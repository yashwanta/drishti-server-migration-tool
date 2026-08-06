package auth

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/drishti/hypershift/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

func testService(t *testing.T, role rbac.Role) *Service {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New([]UserCredential{{User: rbac.User{ID: "user-1", Name: "Test User", Roles: []rbac.Role{role}}, Username: "tester", PasswordHash: string(hash)}}, time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestLoginSessionRBACCSRFAndLogout(t *testing.T) {
	service := testService(t, rbac.RolePlanner)
	if _, _, _, err := service.Login("tester", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("wrong password error = %v", err)
	}
	token, csrf, user, err := service.Login("TESTER", "correct horse battery staple")
	if err != nil || user.ID != "user-1" {
		t.Fatalf("login: user=%#v err=%v", user, err)
	}

	called := false
	handler := service.Require(rbac.ActionCreatePlan, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if contextUser, ok := UserFromContext(r.Context()); !ok || contextUser.ID != user.ID {
			t.Fatalf("missing request identity: %#v, %v", contextUser, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || called {
		t.Fatalf("missing CSRF: status=%d called=%v", w.Code, called)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/plans", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	req.Header.Set("X-CSRF-Token", csrf)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || !called {
		t.Fatalf("authorized request: status=%d called=%v", w.Code, called)
	}

	denied := service.Require(rbac.ActionApprove, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unauthorized handler called") }))
	req = httptest.NewRequest(http.MethodGet, "/denied", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	w = httptest.NewRecorder()
	denied.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("RBAC denial status=%d", w.Code)
	}

	service.delete(token)
	req = httptest.NewRequest(http.MethodGet, "/expired", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("deleted session status=%d", w.Code)
	}
}

func TestSecureCookieAttributes(t *testing.T) {
	service := testService(t, rbac.RoleViewer)
	w := httptest.NewRecorder()
	service.setCookie(w, "opaque")
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" {
		t.Fatalf("unsafe cookie attributes: %#v", cookies)
	}
}

func TestConcurrentSessionAccess(t *testing.T) {
	service := testService(t, rbac.RolePlatformAdmin)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, _, _, err := service.Login("tester", "correct horse battery staple")
			if err != nil {
				t.Errorf("login: %v", err)
				return
			}
			if _, ok := service.lookup(token); !ok {
				t.Error("session disappeared")
			}
			service.delete(token)
		}()
	}
	wg.Wait()
}
