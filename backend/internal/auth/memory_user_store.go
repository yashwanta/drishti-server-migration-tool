package auth

import (
	"context"
	"sort"
	"sync"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/rbac"
)

// MemoryUserStore is the restart-ephemeral user repository used in mock mode.
type MemoryUserStore struct {
	mu    sync.RWMutex
	users map[string]UserRecord
	audit AuditWriter
}

func NewMemoryUserStore(records []UserRecord, audit AuditWriter) (*MemoryUserStore, error) {
	s := &MemoryUserStore{users: make(map[string]UserRecord), audit: audit}
	if _, err := s.BootstrapUsers(context.Background(), records); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *MemoryUserStore) CountUsers(context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users), nil
}

func (s *MemoryUserStore) BootstrapUsers(_ context.Context, records []UserRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) > 0 {
		return false, nil
	}
	for _, record := range records {
		if err := validateStoredUser(record); err != nil {
			return false, err
		}
		username := normalizeUsername(record.Username)
		if _, exists := s.users[username]; exists {
			return false, ErrUsernameExists
		}
		record.Username = username
		record.Roles = append([]rbac.Role(nil), record.Roles...)
		s.users[username] = record
	}
	return true, nil
}

func (s *MemoryUserStore) FindUserByUsername(_ context.Context, username string) (UserRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.users[normalizeUsername(username)]
	record.Roles = append([]rbac.Role(nil), record.Roles...)
	return record, ok, nil
}

func (s *MemoryUserStore) FindUserByID(_ context.Context, id string) (UserRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.users {
		if record.ID == id {
			record.Roles = append([]rbac.Role(nil), record.Roles...)
			return record, true, nil
		}
	}
	return UserRecord{}, false, nil
}

func (s *MemoryUserStore) ListUsers(context.Context) ([]UserRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]UserRecord, 0, len(s.users))
	for _, record := range s.users {
		record.Roles = append([]rbac.Role(nil), record.Roles...)
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Username < result[j].Username })
	return result, nil
}

func (s *MemoryUserStore) CreateUser(ctx context.Context, record UserRecord, events []domain.AuditEvent) error {
	if err := validateStoredUser(record); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	username := normalizeUsername(record.Username)
	if _, exists := s.users[username]; exists {
		return ErrUsernameExists
	}
	for _, event := range events {
		if s.audit != nil {
			if err := s.audit(ctx, event); err != nil {
				return err
			}
		}
	}
	record.Username = username
	record.Roles = append([]rbac.Role(nil), record.Roles...)
	s.users[username] = record
	return nil
}

func (s *MemoryUserStore) DeactivateUser(ctx context.Context, id, actor string, event domain.AuditEvent) (UserRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var username string
	var target UserRecord
	for candidate, record := range s.users {
		if record.ID == id {
			username, target = candidate, record
			break
		}
	}
	if username == "" {
		return UserRecord{}, ErrUserNotFound
	}
	if target.ID == actor {
		return UserRecord{}, ErrSelfDeactivate
	}
	if target.Active && hasRole(target.Roles, rbac.RolePlatformAdmin) {
		activeAdmins := 0
		for _, record := range s.users {
			if record.Active && hasRole(record.Roles, rbac.RolePlatformAdmin) {
				activeAdmins++
			}
		}
		if activeAdmins <= 1 {
			return UserRecord{}, ErrLastPlatformAdmin
		}
	}
	if !target.Active {
		return target, nil
	}
	if s.audit != nil {
		if err := s.audit(ctx, event); err != nil {
			return UserRecord{}, err
		}
	}
	target.Active = false
	target.UpdatedAt = event.Timestamp
	s.users[username] = target
	return target, nil
}

func (s *MemoryUserStore) UpdatePassword(ctx context.Context, id, passwordHash string, event domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for username, record := range s.users {
		if record.ID != id {
			continue
		}
		if s.audit != nil {
			if err := s.audit(ctx, event); err != nil {
				return err
			}
		}
		record.PasswordHash = passwordHash
		record.UpdatedAt = event.Timestamp
		s.users[username] = record
		return nil
	}
	return ErrUserNotFound
}
