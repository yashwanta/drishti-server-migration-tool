package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/job"
	_ "github.com/lib/pq"
)

// Postgres is the durable repository used by lab, live, and production modes.
// It stores plans, jobs and their ordered steps, and append-only audit events.
type Postgres struct {
	db *sql.DB
}

// OpenPostgres connects, verifies the database, and applies embedded schema
// migrations. The caller owns Close.
func OpenPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("PostgreSQL URL is required")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	p := &Postgres{db: db}
	if err := Up(ctx, Embedded(), p); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate PostgreSQL: %w", err)
	}
	return p, nil
}

func (p *Postgres) Close() error { return p.db.Close() }

func (p *Postgres) EnsureSchemaMigrations(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	return err
}

func (p *Postgres) AppliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list schema migrations: %w", err)
	}
	defer rows.Close()
	applied := map[int]bool{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func (p *Postgres) Apply(ctx context.Context, migration Migration) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, name) VALUES($1, $2) ON CONFLICT (version) DO NOTHING`,
		migration.Version, migration.Name); err != nil {
		return err
	}
	return tx.Commit()
}

// PutPlan inserts or replaces the complete persisted representation of a plan.
func (p *Postgres) PutPlan(ctx context.Context, plan domain.Plan) error {
	storageMaps, networkMaps, err := marshalPlanMaps(plan)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `
INSERT INTO plans (
 id, name, source_vm_id, source_connection_id, target_node_id, target_connection_id,
 target_vm_name, target_vmid, cpu, memory_mb, firmware, disk_format, status, created_by,
 created_at, updated_at, storage_maps, network_maps, preflight_passed,
 source_power_off_approved, strategy
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
ON CONFLICT (id) DO UPDATE SET
 name=EXCLUDED.name, source_vm_id=EXCLUDED.source_vm_id,
 source_connection_id=EXCLUDED.source_connection_id, target_node_id=EXCLUDED.target_node_id,
 target_connection_id=EXCLUDED.target_connection_id, target_vm_name=EXCLUDED.target_vm_name,
 target_vmid=EXCLUDED.target_vmid, cpu=EXCLUDED.cpu, memory_mb=EXCLUDED.memory_mb,
 firmware=EXCLUDED.firmware, disk_format=EXCLUDED.disk_format, status=EXCLUDED.status,
 created_by=EXCLUDED.created_by, updated_at=EXCLUDED.updated_at,
 storage_maps=EXCLUDED.storage_maps, network_maps=EXCLUDED.network_maps,
 preflight_passed=EXCLUDED.preflight_passed,
 source_power_off_approved=EXCLUDED.source_power_off_approved, strategy=EXCLUDED.strategy`,
		plan.ID, plan.Name, plan.SourceVMID, plan.SourceConnID, plan.TargetNodeID, plan.TargetConnID,
		plan.TargetVMName, plan.TargetVMID, plan.CPU, plan.MemoryMB, plan.Firmware, plan.DiskFormat,
		plan.Status, plan.CreatedBy, plan.CreatedAt, plan.UpdatedAt, storageMaps, networkMaps,
		plan.PreflightPassed, plan.SourcePowerOffApproved, plan.Strategy)
	if err != nil {
		return fmt.Errorf("put plan %q: %w", plan.ID, err)
	}
	return nil
}

func marshalPlanMaps(plan domain.Plan) ([]byte, []byte, error) {
	storageMaps, err := json.Marshal(plan.StorageMaps)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal storage maps: %w", err)
	}
	networkMaps, err := json.Marshal(plan.NetworkMaps)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal network maps: %w", err)
	}
	return storageMaps, networkMaps, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

const planColumns = `id, name, source_vm_id, source_connection_id, target_node_id,
target_connection_id, COALESCE(target_vm_name, ''), target_vmid, cpu, memory_mb, firmware,
disk_format, status, created_by, created_at, updated_at, storage_maps, network_maps,
preflight_passed, source_power_off_approved, strategy`

func scanPlan(row rowScanner) (domain.Plan, error) {
	var plan domain.Plan
	var storageMaps, networkMaps []byte
	err := row.Scan(&plan.ID, &plan.Name, &plan.SourceVMID, &plan.SourceConnID,
		&plan.TargetNodeID, &plan.TargetConnID, &plan.TargetVMName, &plan.TargetVMID,
		&plan.CPU, &plan.MemoryMB, &plan.Firmware, &plan.DiskFormat, &plan.Status,
		&plan.CreatedBy, &plan.CreatedAt, &plan.UpdatedAt, &storageMaps, &networkMaps,
		&plan.PreflightPassed, &plan.SourcePowerOffApproved, &plan.Strategy)
	if err != nil {
		return domain.Plan{}, err
	}
	if err := json.Unmarshal(storageMaps, &plan.StorageMaps); err != nil {
		return domain.Plan{}, fmt.Errorf("decode plan storage maps: %w", err)
	}
	if err := json.Unmarshal(networkMaps, &plan.NetworkMaps); err != nil {
		return domain.Plan{}, fmt.Errorf("decode plan network maps: %w", err)
	}
	return plan, nil
}

func (p *Postgres) GetPlan(ctx context.Context, id string) (domain.Plan, bool, error) {
	plan, err := scanPlan(p.db.QueryRowContext(ctx, `SELECT `+planColumns+` FROM plans WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Plan{}, false, nil
	}
	if err != nil {
		return domain.Plan{}, false, fmt.Errorf("get plan %q: %w", id, err)
	}
	return plan, true, nil
}

func (p *Postgres) ListPlans(ctx context.Context) ([]domain.Plan, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT `+planColumns+` FROM plans ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()
	var plans []domain.Plan
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

func (p *Postgres) DeletePlan(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM plans WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete plan %q: %w", id, err)
	}
	return nil
}

// ApprovePlan records the approval state and its audit event in one database
// transaction. SELECT FOR UPDATE serializes concurrent approvals of one plan.
func (p *Postgres) ApprovePlan(ctx context.Context, id string, approvePowerOff bool, event domain.AuditEvent) (domain.Plan, bool, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return domain.Plan{}, false, err
	}
	defer tx.Rollback()
	plan, err := scanPlan(tx.QueryRowContext(ctx, `SELECT `+planColumns+` FROM plans WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Plan{}, false, nil
	}
	if err != nil {
		return domain.Plan{}, false, err
	}
	if plan.Status != domain.PlanPreflight || !plan.PreflightPassed {
		return plan, true, fmt.Errorf("plan requires a passing preflight before approval")
	}
	plan.Status = domain.PlanApproved
	plan.SourcePowerOffApproved = approvePowerOff
	plan.UpdatedAt = event.Timestamp
	if _, err := tx.ExecContext(ctx, `UPDATE plans SET status=$2, source_power_off_approved=$3, updated_at=$4 WHERE id=$1`,
		plan.ID, plan.Status, plan.SourcePowerOffApproved, plan.UpdatedAt); err != nil {
		return domain.Plan{}, false, err
	}
	if err := appendAudit(ctx, tx, event); err != nil {
		return domain.Plan{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Plan{}, false, err
	}
	return plan, true, nil
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func appendAudit(ctx context.Context, db execer, event domain.AuditEvent) error {
	_, err := db.ExecContext(ctx, `INSERT INTO audit_events(id, ts, actor, action, target, result, detail)
VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`, event.ID, event.Timestamp,
		event.Actor, event.Action, event.Target, event.Result, event.Detail)
	return err
}

func (p *Postgres) AppendAudit(ctx context.Context, event domain.AuditEvent) error {
	if err := appendAudit(ctx, p.db, event); err != nil {
		return fmt.Errorf("append audit event %q: %w", event.ID, err)
	}
	return nil
}

func (p *Postgres) ListAudit(ctx context.Context) ([]domain.AuditEvent, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, ts, actor, action, target, result, COALESCE(detail, '')
FROM audit_events ORDER BY ts, id`)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	var events []domain.AuditEvent
	for rows.Next() {
		var event domain.AuditEvent
		if err := rows.Scan(&event.ID, &event.Timestamp, &event.Actor, &event.Action, &event.Target, &event.Result, &event.Detail); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// SaveJob atomically replaces a job and its ordered steps.
func (p *Postgres) SaveJob(ctx context.Context, value *job.Job) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	actor := value.Actor
	if actor == "" {
		actor = "system"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO jobs(id, plan_id, actor, state, idempotency_key, started_at, finished_at)
VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO UPDATE SET actor=EXCLUDED.actor, state=EXCLUDED.state,
idempotency_key=EXCLUDED.idempotency_key, started_at=EXCLUDED.started_at, finished_at=EXCLUDED.finished_at`,
		value.ID, value.PlanID, actor, value.State, value.IdempotencyKey, value.StartedAt, value.FinishedAt)
	if err != nil {
		return fmt.Errorf("save job %q: %w", value.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM job_steps WHERE job_id=$1`, value.ID); err != nil {
		return err
	}
	for i, step := range value.Steps {
		stepID := step.ID
		if stepID == "" {
			stepID = fmt.Sprintf("%s-step-%02d", value.ID, i)
		}
		stepActor := step.Actor
		if stepActor == "" {
			stepActor = actor
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO job_steps
(id, job_id, name, actor, state, external_id, message, started_at, finished_at, step_order)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, stepID, value.ID, step.Name, stepActor, step.State,
			step.ExternalID, step.Message, step.StartedAt, step.FinishedAt, i); err != nil {
			return fmt.Errorf("save job step %q: %w", step.Name, err)
		}
	}
	return tx.Commit()
}

func scanJob(row rowScanner) (*job.Job, error) {
	value := &job.Job{}
	if err := row.Scan(&value.ID, &value.PlanID, &value.Actor, &value.State, &value.IdempotencyKey, &value.StartedAt, &value.FinishedAt); err != nil {
		return nil, err
	}
	return value, nil
}

func (p *Postgres) loadSteps(ctx context.Context, value *job.Job) error {
	rows, err := p.db.QueryContext(ctx, `SELECT id, name, actor, state, COALESCE(external_id, ''), COALESCE(message, ''), started_at, finished_at
FROM job_steps WHERE job_id=$1 ORDER BY step_order`, value.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var step job.Step
		if err := rows.Scan(&step.ID, &step.Name, &step.Actor, &step.State, &step.ExternalID, &step.Message, &step.StartedAt, &step.FinishedAt); err != nil {
			return err
		}
		value.Steps = append(value.Steps, step)
	}
	return rows.Err()
}

func (p *Postgres) GetJob(ctx context.Context, id string) (*job.Job, bool, error) {
	value, err := scanJob(p.db.QueryRowContext(ctx, `SELECT id, plan_id, actor, state, idempotency_key, started_at, finished_at FROM jobs WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get job %q: %w", id, err)
	}
	if err := p.loadSteps(ctx, value); err != nil {
		return nil, false, fmt.Errorf("load job steps %q: %w", id, err)
	}
	return value, true, nil
}

func (p *Postgres) ListJobs(ctx context.Context) ([]*job.Job, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, plan_id, actor, state, idempotency_key, started_at, finished_at FROM jobs ORDER BY started_at NULLS FIRST, id`)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()
	var jobs []*job.Job
	for rows.Next() {
		value, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, value := range jobs {
		if err := p.loadSteps(ctx, value); err != nil {
			return nil, err
		}
	}
	return jobs, nil
}
