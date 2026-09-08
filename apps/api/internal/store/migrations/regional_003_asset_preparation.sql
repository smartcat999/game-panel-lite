ALTER TABLE regional_revision_tasks ADD COLUMN asset_lease_token text NOT NULL DEFAULT '';
ALTER TABLE regional_revision_tasks ADD COLUMN asset_lease_until_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_revision_tasks ADD COLUMN asset_next_attempt_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_revision_tasks ADD COLUMN asset_attempts bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_revision_tasks DROP CONSTRAINT regional_revision_tasks_status_check;
ALTER TABLE regional_revision_tasks ADD CONSTRAINT regional_revision_tasks_status_check CHECK(status IN ('awaiting_revision','revision_fetched','assets_prepared','materialized','rejected'));
CREATE INDEX idx_regional_assets_ready ON regional_revision_tasks(asset_next_attempt_ms,created_at,operation_id) WHERE status = 'revision_fetched';
