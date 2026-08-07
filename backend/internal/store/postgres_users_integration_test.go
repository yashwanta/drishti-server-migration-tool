package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

func TestPostgresUsersBootstrapDurabilityAuditAndDeactivationPolicies(t *testing.T) {
	databaseURL := os.Getenv("DRISHTI_TEST_DB_URL")
	if databaseURL == "" {
		t.Skip("DRISHTI_TEST_DB_URL is not set")
	}
	ctx := context.Background()
	database, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.db.ExecContext(ctx, `TRUNCATE auth_users, audit_events RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("bootstrap-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	bootstrap := []auth.UserRecord{
		{ID: "admin-1", Username: "admin-one", Name: "Admin One", Roles: []rbac.Role{rbac.RolePlatformAdmin}, Active: true, PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now},
		{ID: "admin-2", Username: "admin-two", Name: "Admin Two", Roles: []rbac.Role{rbac.RolePlatformAdmin}, Active: true, PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now},
	}
	imported, err := database.BootstrapUsers(ctx, bootstrap)
	if err != nil || !imported {
		t.Fatalf("first bootstrap imported=%v err=%v", imported, err)
	}
	replacement := []auth.UserRecord{{ID: "replacement", Username: "replacement", Name: "Replacement", Roles: []rbac.Role{rbac.RolePlatformAdmin}, Active: true, PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now}}
	imported, err = database.BootstrapUsers(ctx, replacement)
	if err != nil || imported {
		t.Fatalf("second bootstrap imported=%v err=%v", imported, err)
	}
	if _, found, err := database.FindUserByUsername(ctx, "replacement"); err != nil || found {
		t.Fatalf("bootstrap overwrote authoritative database: found=%v err=%v", found, err)
	}

	user := auth.UserRecord{ID: "operator-1", Username: "operator-one", Name: "Operator One", Roles: []rbac.Role{rbac.RoleOperator}, Active: true, PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now}
	events := []domain.AuditEvent{
		{ID: "evt-user-create", Timestamp: now, Actor: "admin-1", Action: "user.create", Target: user.ID, Result: "created"},
		{ID: "evt-role-assign", Timestamp: now, Actor: "admin-1", Action: "user.roles.assign", Target: user.ID, Result: "assigned"},
	}
	if err := database.CreateUser(ctx, user, events); err != nil {
		t.Fatal(err)
	}
	secondConnection, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer secondConnection.Close()
	persisted, found, err := secondConnection.FindUserByUsername(ctx, "OPERATOR-ONE")
	if err != nil || !found || persisted.ID != user.ID {
		t.Fatalf("reopened user=%#v found=%v err=%v", persisted, found, err)
	}
	audits, err := secondConnection.ListAudit(ctx)
	if err != nil || len(audits) != 2 || audits[0].Actor != "admin-1" {
		t.Fatalf("audits=%#v err=%v", audits, err)
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte("replacement-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	passwordEvent := domain.AuditEvent{ID: "evt-password", Timestamp: now.Add(time.Second), Actor: user.ID, Action: "user.password_change", Target: user.ID, Result: "changed"}
	if err := secondConnection.UpdatePassword(ctx, user.ID, string(newHash), passwordEvent); err != nil {
		t.Fatal(err)
	}
	persisted, _, _ = secondConnection.FindUserByID(ctx, user.ID)
	if bcrypt.CompareHashAndPassword([]byte(persisted.PasswordHash), []byte("replacement-password")) != nil {
		t.Fatal("updated bcrypt hash did not persist")
	}

	selfEvent := domain.AuditEvent{ID: "evt-self", Timestamp: now, Actor: "admin-1", Action: "user.deactivate", Target: "admin-1"}
	if _, err := secondConnection.DeactivateUser(ctx, "admin-1", "admin-1", selfEvent); !errors.Is(err, auth.ErrSelfDeactivate) {
		t.Fatalf("self deactivation err=%v", err)
	}
	secondAdminEvent := domain.AuditEvent{ID: "evt-admin-two", Timestamp: now.Add(2 * time.Second), Actor: "admin-1", Action: "user.deactivate", Target: "admin-2", Result: "deactivated"}
	if _, err := secondConnection.DeactivateUser(ctx, "admin-2", "admin-1", secondAdminEvent); err != nil {
		t.Fatal(err)
	}
	lastEvent := domain.AuditEvent{ID: "evt-last", Timestamp: now.Add(3 * time.Second), Actor: "external-admin", Action: "user.deactivate", Target: "admin-1"}
	if _, err := secondConnection.DeactivateUser(ctx, "admin-1", "external-admin", lastEvent); !errors.Is(err, auth.ErrLastPlatformAdmin) {
		t.Fatalf("last admin deactivation err=%v", err)
	}
}
