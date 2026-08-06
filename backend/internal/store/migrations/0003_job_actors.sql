-- Preserve the authenticated initiator on durable jobs and every ordered step.
-- This is audit metadata only and enables no migration or hypervisor mutation.

ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS actor TEXT NOT NULL DEFAULT 'system';

ALTER TABLE job_steps
    ADD COLUMN IF NOT EXISTS actor TEXT NOT NULL DEFAULT 'system';
