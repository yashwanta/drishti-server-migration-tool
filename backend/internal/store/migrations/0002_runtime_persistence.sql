-- Persist the complete runtime plan and ordered job-step models.
-- This migration is additive and contains no hypervisor or source-VM actions.

ALTER TABLE plans
    ADD COLUMN IF NOT EXISTS storage_maps JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS network_maps JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS preflight_passed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS source_power_off_approved BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS strategy TEXT NOT NULL DEFAULT 'cold';

ALTER TABLE job_steps
    ADD COLUMN IF NOT EXISTS step_order INTEGER NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_idempotency_key
    ON jobs(idempotency_key);

CREATE UNIQUE INDEX IF NOT EXISTS idx_job_steps_job_order
    ON job_steps(job_id, step_order);
