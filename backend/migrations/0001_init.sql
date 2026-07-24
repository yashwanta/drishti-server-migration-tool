-- DRISHTI HyperShift schema baseline (Phase 0).
-- This migration creates only the tables required to persist connections,
-- plans, jobs, preflight checks, and audit events. No destructive hypervisor
-- operation is represented here.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS connections (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('vmware','proxmox')),
    role          TEXT NOT NULL CHECK (role IN ('source','target')),
    endpoint      TEXT NOT NULL,
    insecure_tls  BOOLEAN NOT NULL DEFAULT FALSE,
    status        TEXT NOT NULL DEFAULT 'unknown',
    -- Secrets are referenced, never stored inline.
    secret_ref    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plans (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    source_vm_id        TEXT NOT NULL,
    source_connection_id TEXT NOT NULL,
    target_node_id      TEXT NOT NULL,
    target_connection_id TEXT NOT NULL,
    target_vm_name      TEXT,
    target_vmid         INTEGER,
    cpu                 INTEGER NOT NULL DEFAULT 1,
    memory_mb           BIGINT NOT NULL DEFAULT 512,
    firmware            TEXT NOT NULL DEFAULT 'bios',
    disk_format         TEXT NOT NULL DEFAULT 'qcow2',
    status              TEXT NOT NULL DEFAULT 'draft',
    created_by          TEXT NOT NULL DEFAULT 'system',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS jobs (
    id               TEXT PRIMARY KEY,
    plan_id          TEXT NOT NULL REFERENCES plans(id),
    state            TEXT NOT NULL DEFAULT 'pending',
    idempotency_key  TEXT NOT NULL,
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS job_steps (
    id          TEXT PRIMARY KEY,
    job_id      TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT 'pending',
    external_id TEXT,
    message     TEXT,
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS audit_events (
    id         TEXT PRIMARY KEY,
    ts         TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor      TEXT NOT NULL,
    action     TEXT NOT NULL,
    target     TEXT NOT NULL,
    result     TEXT NOT NULL,
    detail     TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_events(ts);
CREATE INDEX IF NOT EXISTS idx_jobs_plan ON jobs(plan_id);
