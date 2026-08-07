// Package auth provides password authentication, server-side sessions, CSRF
// protection, and request identity propagation.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/drishti/hypershift/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

const CookieName = "drishti_session"

var ErrInvalidCredentials = errors.New("invalid credentials")

// UserCredential is one configured login. PasswordHash must be bcrypt; a
// plaintext password is never accepted by configuration.
type UserCredential struct {
	rbac.User
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

type userFile struct {
	Users []UserCredential `json:"users"`
}

type session struct {
	User      rbac.User
	CSRFToken string
	ExpiresAt time.Time
}

// Service owns configured users and hashed, process-local session tokens.
// Raw session tokens exist only in the HttpOnly cookie.
type Service struct {
	mu        sync.RWMutex
	users     UserStore
	sessions  map[[32]byte]session
	ttl       time.Duration
	secure    bool
	now       func() time.Time
	dummyHash []byte
}

// LoadUsers loads bcrypt password hashes and RBAC roles from a protected JSON file.
func LoadUsers(path string, ttl time.Duration, secure bool) (*Service, error) {
	records, err := LoadUserRecords(path)
	if err != nil {
		return nil, err
	}
	return New(records, ttl, secure)
}

// LoadUserRecords reads and validates the protected JSON seed/bootstrap file.
func LoadUserRecords(path string) ([]UserCredential, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("DRISHTI_AUTH_USERS_FILE is required for mock mode or first database bootstrap")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read auth users file: %w", err)
	}
	var file userFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode auth users file: %w", err)
	}
	if len(file.Users) == 0 {
		return nil, errors.New("auth users file must contain at least one user")
	}
	return file.Users, nil
}

// New creates a service from already-loaded records. It is exported for tests
// and controlled embedding; production startup uses LoadUsers.
func New(records []UserCredential, ttl time.Duration, secure bool) (*Service, error) {
	stored, err := RecordsFromCredentials(records, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	memory, err := NewMemoryUserStore(stored, nil)
	if err != nil {
		return nil, err
	}
	return NewWithStore(memory, ttl, secure)
}

// NewWithStore creates the authentication service around the selected runtime
// repository. PostgreSQL is supplied in real modes; mock mode supplies memory.
func NewWithStore(users UserStore, ttl time.Duration, secure bool) (*Service, error) {
	if ttl <= 0 {
		return nil, errors.New("session TTL must be positive")
	}
	if users == nil {
		return nil, errors.New("user store is required")
	}
	dummyHash, err := bcrypt.GenerateFromPassword([]byte("invalid-login-sentinel"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("initialize password verifier: %w", err)
	}
	s := &Service{users: users, sessions: make(map[[32]byte]session), ttl: ttl, secure: secure, now: time.Now, dummyHash: dummyHash}
	return s, nil
}

func validRole(role rbac.Role) bool {
	switch role {
	case rbac.RoleViewer, rbac.RolePlanner, rbac.RoleOperator, rbac.RoleApprover, rbac.RolePlatformAdmin, rbac.RoleAuditor:
		return true
	default:
		return false
	}
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func tokenKey(raw string) [32]byte { return sha256.Sum256([]byte(raw)) }

// Login validates a password and creates a fresh server-side session.
func (s *Service) Login(username, password string) (string, string, rbac.User, error) {
	record, ok, storeErr := s.users.FindUserByUsername(context.Background(), username)
	if storeErr != nil {
		return "", "", rbac.User{}, fmt.Errorf("lookup user: %w", storeErr)
	}
	hash := s.dummyHash
	if ok {
		hash = []byte(record.PasswordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if !ok || !record.Active || passwordErr != nil {
		return "", "", rbac.User{}, ErrInvalidCredentials
	}
	token, err := randomToken()
	if err != nil {
		return "", "", rbac.User{}, fmt.Errorf("generate session token: %w", err)
	}
	csrf, err := randomToken()
	if err != nil {
		return "", "", rbac.User{}, fmt.Errorf("generate CSRF token: %w", err)
	}
	s.mu.Lock()
	user := record.sessionUser()
	s.sessions[tokenKey(token)] = session{User: user, CSRFToken: csrf, ExpiresAt: s.now().Add(s.ttl)}
	s.mu.Unlock()
	return token, csrf, user, nil
}

func (s *Service) lookup(raw string) (session, bool) {
	key := tokenKey(raw)
	s.mu.RLock()
	sess, ok := s.sessions[key]
	s.mu.RUnlock()
	if !ok {
		return session{}, false
	}
	if !s.now().Before(sess.ExpiresAt) {
		s.mu.Lock()
		delete(s.sessions, key)
		s.mu.Unlock()
		return session{}, false
	}
	return sess, true
}

func (s *Service) delete(raw string) {
	s.mu.Lock()
	delete(s.sessions, tokenKey(raw))
	s.mu.Unlock()
}

func (s *Service) invalidateUserSessions(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, sess := range s.sessions {
		if sess.User.ID == userID {
			delete(s.sessions, key)
		}
	}
}

func (s *Service) setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: int(s.ttl.Seconds())})
}

func (s *Service) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

type contextKey struct{}

// UserFromContext returns the authenticated request user.
func UserFromContext(ctx context.Context) (rbac.User, bool) {
	user, ok := ctx.Value(contextKey{}).(rbac.User)
	return user, ok
}

// Actor returns a stable audit identity without exposing session material.
func Actor(ctx context.Context) string {
	if user, ok := UserFromContext(ctx); ok {
		return user.ID
	}
	return "unauthenticated"
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResponse struct {
	User      rbac.User `json:"user"`
	CSRFToken string    `json:"csrf_token"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// LoginHandler is the sole unauthenticated API endpoint.
func (s *Service) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	token, csrf, user, err := s.Login(input.Username, input.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	s.setCookie(w, token)
	writeJSON(w, http.StatusOK, sessionResponse{User: user, CSRFToken: csrf})
}

func (s *Service) authenticate(r *http.Request) (session, string, bool) {
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return session{}, "", false
	}
	sess, ok := s.lookup(cookie.Value)
	return sess, cookie.Value, ok
}

// Require enforces a valid session, CSRF for unsafe methods, and an RBAC action.
func (s *Service) Require(action rbac.Action, next http.Handler) http.Handler {
	return s.require(&action, next)
}

// RequireSession enforces authentication without requiring a domain action.
func (s *Service) RequireSession(next http.Handler) http.Handler { return s.require(nil, next) }

func (s *Service) require(action *rbac.Action, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, _, ok := s.authenticate(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required", "code": "unauthorized"})
			return
		}
		csrfValid := subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(sess.CSRFToken)) == 1
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !csrfValid {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid CSRF token", "code": "csrf_failed"})
			return
		}
		if action != nil && !sess.User.Can(*action) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied", "code": "forbidden"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, sess.User)))
	})
}

// SessionHandler returns the identity and CSRF token for an existing session.
func (s *Service) SessionHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, ok := s.authenticate(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required", "code": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{User: sess.User, CSRFToken: sess.CSRFToken})
}

// LogoutHandler invalidates the server-side session and expires its cookie.
func (s *Service) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	_, raw, ok := s.authenticate(r)
	if ok {
		s.delete(raw)
	}
	s.clearCookie(w)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
