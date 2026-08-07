package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

type createUserRequest struct {
	Username string      `json:"username"`
	Roles    []rbac.Role `json:"roles"`
	Password string      `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func decodeRequest(w http.ResponseWriter, r *http.Request, value any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func newIdentity(prefix string) (string, error) {
	value, err := randomToken()
	if err != nil {
		return "", err
	}
	return prefix + value, nil
}

func validUsername(value string) bool {
	value = normalizeUsername(value)
	if len(value) < 3 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

// ListUsersHandler returns only the safe administrative user projection.
func (s *Service) ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	records, err := s.users.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "user store unavailable", "code": "store_unavailable"})
		return
	}
	users := make([]ManagedUser, 0, len(records))
	for _, record := range records {
		users = append(users, record.public())
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// CreateUserHandler creates a bcrypt-backed login and atomically audits both
// the user creation and the assigned roles.
func (s *Service) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var input createUserRequest
	if err := decodeRequest(w, r, &input); err != nil || !validUsername(input.Username) || validateRoles(input.Roles) != nil || validatePassword(input.Password) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), PasswordCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user creation failed"})
		return
	}
	userID, err := newIdentity("user-")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user creation failed"})
		return
	}
	createEventID, err := newIdentity("evt-")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user creation failed"})
		return
	}
	rolesEventID, err := newIdentity("evt-")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user creation failed"})
		return
	}
	now := time.Now().UTC()
	username := normalizeUsername(input.Username)
	record := UserRecord{ID: userID, Username: username, Name: username, Roles: append([]rbac.Role(nil), input.Roles...), Active: true, PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now}
	actor := Actor(r.Context())
	events := []domain.AuditEvent{
		{ID: createEventID, Timestamp: now, Actor: actor, Action: "user.create", Target: userID, Result: "created", Detail: fmt.Sprintf("Created user %s.", username)},
		{ID: rolesEventID, Timestamp: now, Actor: actor, Action: "user.roles.assign", Target: userID, Result: "assigned", Detail: fmt.Sprintf("Assigned roles: %s.", roleNames(input.Roles))},
	}
	if err := s.users.CreateUser(r.Context(), record, events); err != nil {
		if errors.Is(err, ErrUsernameExists) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists", "code": "conflict"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "user store unavailable", "code": "store_unavailable"})
		return
	}
	writeJSON(w, http.StatusCreated, record.public())
}

func roleNames(roles []rbac.Role) string {
	values := make([]string, len(roles))
	for i, role := range roles {
		values[i] = string(role)
	}
	return strings.Join(values, ", ")
}

// DeactivateUserHandler disables a user without deleting its audit identity.
func (s *Service) DeactivateUserHandler(w http.ResponseWriter, r *http.Request) {
	targetID := strings.TrimSpace(r.PathValue("id"))
	actor := Actor(r.Context())
	eventID, err := newIdentity("evt-")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user deactivation failed"})
		return
	}
	event := domain.AuditEvent{ID: eventID, Timestamp: time.Now().UTC(), Actor: actor, Action: "user.deactivate", Target: targetID, Result: "deactivated", Detail: "User login disabled and active sessions invalidated."}
	record, err := s.users.DeactivateUser(r.Context(), targetID, actor, event)
	if err != nil {
		switch {
		case errors.Is(err, ErrUserNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found", "code": "not_found"})
		case errors.Is(err, ErrSelfDeactivate), errors.Is(err, ErrLastPlatformAdmin):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "protected_admin"})
		default:
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "user store unavailable", "code": "store_unavailable"})
		}
		return
	}
	s.invalidateUserSessions(record.ID)
	writeJSON(w, http.StatusOK, record.public())
}

// ChangePasswordHandler verifies the current secret, stores a fresh bcrypt
// hash, and rotates every active session for the authenticated user.
func (s *Service) ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	var input changePasswordRequest
	if err := decodeRequest(w, r, &input); err != nil || validatePassword(input.NewPassword) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	record, found, err := s.users.FindUserByID(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "user store unavailable", "code": "store_unavailable"})
		return
	}
	if !found || !record.Active || bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(input.CurrentPassword)) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), PasswordCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password change failed"})
		return
	}
	eventID, err := newIdentity("evt-")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password change failed"})
		return
	}
	event := domain.AuditEvent{ID: eventID, Timestamp: time.Now().UTC(), Actor: user.ID, Action: "user.password_change", Target: user.ID, Result: "changed", Detail: "Password changed; active sessions invalidated."}
	if err := s.users.UpdatePassword(r.Context(), user.ID, string(hash), event); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "user store unavailable", "code": "store_unavailable"})
		return
	}
	s.invalidateUserSessions(user.ID)
	s.clearCookie(w)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
