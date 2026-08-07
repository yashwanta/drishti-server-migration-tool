package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/rbac"
	"github.com/lib/pq"
)

const userColumns = `id, username, display_name, password_hash, roles, active, created_at, updated_at`

func scanUser(row rowScanner) (auth.UserRecord, error) {
	var record auth.UserRecord
	var roles []byte
	if err := row.Scan(&record.ID, &record.Username, &record.Name, &record.PasswordHash, &roles, &record.Active, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return auth.UserRecord{}, err
	}
	if err := json.Unmarshal(roles, &record.Roles); err != nil {
		return auth.UserRecord{}, fmt.Errorf("decode user roles: %w", err)
	}
	return record, nil
}

func marshalRoles(roles []rbac.Role) ([]byte, error) {
	value, err := json.Marshal(roles)
	if err != nil {
		return nil, fmt.Errorf("encode user roles: %w", err)
	}
	return value, nil
}

func insertUser(ctx context.Context, db execer, record auth.UserRecord) error {
	roles, err := marshalRoles(record.Roles)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO auth_users
(id, username, display_name, password_hash, roles, active, created_at, updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, record.ID, record.Username, record.Name, record.PasswordHash, roles, record.Active, record.CreatedAt, record.UpdatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return auth.ErrUsernameExists
		}
		return err
	}
	return nil
}

func (p *Postgres) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := p.db.QueryRowContext(ctx, `SELECT count(*) FROM auth_users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count auth users: %w", err)
	}
	return count, nil
}

// BootstrapUsers imports the protected JSON records only when the table is
// empty. The table lock makes concurrent first starts converge on one import.
func (p *Postgres) BootstrapUsers(ctx context.Context, records []auth.UserRecord) (bool, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `LOCK TABLE auth_users IN EXCLUSIVE MODE`); err != nil {
		return false, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM auth_users`).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, tx.Commit()
	}
	activeAdmin := false
	for _, record := range records {
		if record.Active {
			for _, role := range record.Roles {
				if role == rbac.RolePlatformAdmin {
					activeAdmin = true
				}
			}
		}
		if err := insertUser(ctx, tx, record); err != nil {
			return false, err
		}
	}
	if !activeAdmin {
		return false, fmt.Errorf("initial auth users must include an active platform_admin")
	}
	return true, tx.Commit()
}

func (p *Postgres) FindUserByUsername(ctx context.Context, username string) (auth.UserRecord, bool, error) {
	record, err := scanUser(p.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM auth_users WHERE username=$1`, strings.ToLower(strings.TrimSpace(username))))
	if errors.Is(err, sql.ErrNoRows) {
		return auth.UserRecord{}, false, nil
	}
	if err != nil {
		return auth.UserRecord{}, false, fmt.Errorf("find auth user: %w", err)
	}
	return record, true, nil
}

func (p *Postgres) FindUserByID(ctx context.Context, id string) (auth.UserRecord, bool, error) {
	record, err := scanUser(p.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM auth_users WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return auth.UserRecord{}, false, nil
	}
	if err != nil {
		return auth.UserRecord{}, false, fmt.Errorf("get auth user: %w", err)
	}
	return record, true, nil
}

func (p *Postgres) ListUsers(ctx context.Context) ([]auth.UserRecord, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT `+userColumns+` FROM auth_users ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("list auth users: %w", err)
	}
	defer rows.Close()
	var records []auth.UserRecord
	for rows.Next() {
		record, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (p *Postgres) CreateUser(ctx context.Context, record auth.UserRecord, events []domain.AuditEvent) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertUser(ctx, tx, record); err != nil {
		return err
	}
	for _, event := range events {
		if err := appendAudit(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *Postgres) DeactivateUser(ctx context.Context, id, actor string, event domain.AuditEvent) (auth.UserRecord, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return auth.UserRecord{}, err
	}
	defer tx.Rollback()
	record, err := scanUser(tx.QueryRowContext(ctx, `SELECT `+userColumns+` FROM auth_users WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return auth.UserRecord{}, auth.ErrUserNotFound
	}
	if err != nil {
		return auth.UserRecord{}, err
	}
	if record.ID == actor {
		return auth.UserRecord{}, auth.ErrSelfDeactivate
	}
	if !record.Active {
		return record, tx.Commit()
	}
	if hasPlatformAdmin(record.Roles) {
		var activeAdmins int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM auth_users WHERE active AND roles @> '["platform_admin"]'::jsonb`).Scan(&activeAdmins); err != nil {
			return auth.UserRecord{}, err
		}
		if activeAdmins <= 1 {
			return auth.UserRecord{}, auth.ErrLastPlatformAdmin
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auth_users SET active=FALSE, updated_at=$2 WHERE id=$1`, id, event.Timestamp); err != nil {
		return auth.UserRecord{}, err
	}
	if err := appendAudit(ctx, tx, event); err != nil {
		return auth.UserRecord{}, err
	}
	record.Active = false
	record.UpdatedAt = event.Timestamp
	return record, tx.Commit()
}

func hasPlatformAdmin(roles []rbac.Role) bool {
	for _, role := range roles {
		if role == rbac.RolePlatformAdmin {
			return true
		}
	}
	return false
}

func (p *Postgres) UpdatePassword(ctx context.Context, id, passwordHash string, event domain.AuditEvent) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE auth_users SET password_hash=$2, updated_at=$3 WHERE id=$1 AND active`, id, passwordHash, event.Timestamp)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return auth.ErrUserNotFound
	}
	if err := appendAudit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}
