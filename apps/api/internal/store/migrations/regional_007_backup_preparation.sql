ALTER TABLE regional_backup_requests ADD COLUMN lease_token text NOT NULL DEFAULT '';
ALTER TABLE regional_backup_requests ADD COLUMN lease_until_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_backup_requests ADD COLUMN next_attempt_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_backup_requests ADD COLUMN attempts bigint NOT NULL DEFAULT 0;
CREATE INDEX idx_regional_backup_preparation_ready ON regional_backup_requests(next_attempt_ms,created_at,operation_id) WHERE status = 'awaiting_authority';
