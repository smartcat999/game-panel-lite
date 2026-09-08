ALTER TABLE regional_revision_tasks ADD COLUMN lease_token text NOT NULL DEFAULT '';
ALTER TABLE regional_revision_tasks ADD COLUMN lease_until_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_revision_tasks ADD COLUMN next_attempt_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_revision_tasks ADD COLUMN attempts bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_revision_tasks ADD COLUMN snapshot text;
ALTER TABLE regional_revision_tasks DROP CONSTRAINT regional_revision_tasks_status_check;
ALTER TABLE regional_revision_tasks ADD CONSTRAINT regional_revision_tasks_status_check CHECK(status IN ('awaiting_revision','revision_fetched','materialized','rejected'));
CREATE INDEX idx_regional_revision_fetch_ready ON regional_revision_tasks(next_attempt_ms,created_at,operation_id) WHERE status = 'awaiting_revision';
