package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

// PasswordCost is used for every server-generated operator password hash.
const PasswordCost = bcrypt.DefaultCost

const (
	minPasswordBytes = 12
	maxPasswordBytes = 72
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUsernameExists    = errors.New("username already exists")
	ErrSelfDeactivate    = errors.New("administrators cannot deactivate themselves")
	ErrLastPlatformAdmin = errors.New("the last active platform administrator cannot be deactivated")
	ErrInvalidUser       = errors.New("invalid user")
)

// UserRecord is the internal authentication record. PasswordHash is never
// serialized into an API response.
type UserRecord struct {
	ID           string      `json:"id"`
	Username     string      `json:"username"`
	Name         string      `json:"name"`
	Roles        []rbac.Role `json:"roles"`
	Active       bool        `json:"active"`
	PasswordHash string      `json:"-"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// ManagedUser is the safe administrative representation of a user.
type ManagedUser struct {
	ID        string      `json:"id"`
	Username  string      `json:"username"`
	Name      string      `json:"name"`
	Roles     []rbac.Role `json:"roles"`
	Active    bool        `json:"active"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

func (u UserRecord) public() ManagedUser {
	roles := append([]rbac.Role(nil), u.Roles...)
	return ManagedUser{ID: u.ID, Username: u.Username, Name: u.Name, Roles: roles, Active: u.Active, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

func (u UserRecord) sessionUser() rbac.User {
	return rbac.User{ID: u.ID, Name: u.Name, Roles: append([]rbac.Role(nil), u.Roles...)}
}

// RecordsFromCredentials converts protected JSON records into active store
// records for mock seeding or a one-time PostgreSQL bootstrap.
func RecordsFromCredentials(records []UserCredential, now time.Time) ([]UserRecord, error) {
	if len(records) == 0 {
		return nil, ErrInvalidUser
	}
	result := make([]UserRecord, 0, len(records))
	for _, record := range records {
		value := UserRecord{ID: strings.TrimSpace(record.ID), Username: normalizeUsername(record.Username), Name: strings.TrimSpace(record.Name), Roles: append([]rbac.Role(nil), record.Roles...), Active: true, PasswordHash: record.PasswordHash, CreatedAt: now, UpdatedAt: now}
		if value.Name == "" {
			value.Name = value.Username
		}
		if err := validateStoredUser(value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

// UserStore is implemented by memory in mock mode and PostgreSQL in every
// real mode. Mutations and their supplied audit events must be atomic.
type UserStore interface {
	CountUsers(context.Context) (int, error)
	BootstrapUsers(context.Context, []UserRecord) (bool, error)
	FindUserByUsername(context.Context, string) (UserRecord, bool, error)
	FindUserByID(context.Context, string) (UserRecord, bool, error)
	ListUsers(context.Context) ([]UserRecord, error)
	CreateUser(context.Context, UserRecord, []domain.AuditEvent) error
	DeactivateUser(context.Context, string, string, domain.AuditEvent) (UserRecord, error)
	UpdatePassword(context.Context, string, string, domain.AuditEvent) error
}

// AuditWriter appends a sanitized identity audit event.
type AuditWriter func(context.Context, domain.AuditEvent) error

func normalizeUsername(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func validateRoles(roles []rbac.Role) error {
	if len(roles) == 0 {
		return ErrInvalidUser
	}
	seen := make(map[rbac.Role]bool, len(roles))
	for _, role := range roles {
		if !validRole(role) || seen[role] {
			return ErrInvalidUser
		}
		seen[role] = true
	}
	return nil
}

func validatePassword(password string) error {
	if !utf8.ValidString(password) || len([]byte(password)) < minPasswordBytes || len([]byte(password)) > maxPasswordBytes {
		return ErrInvalidUser
	}
	return nil
}

func validateStoredUser(record UserRecord) error {
	if strings.TrimSpace(record.ID) == "" || normalizeUsername(record.Username) == "" || strings.TrimSpace(record.Name) == "" {
		return ErrInvalidUser
	}
	if err := validateRoles(record.Roles); err != nil {
		return err
	}
	if _, err := bcrypt.Cost([]byte(record.PasswordHash)); err != nil {
		return ErrInvalidUser
	}
	return nil
}

func hasRole(roles []rbac.Role, wanted rbac.Role) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}
