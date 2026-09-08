ALTER TABLE regional_backup_result_outbox ADD COLUMN lease_token text NOT NULL DEFAULT '';
ALTER TABLE regional_backup_result_outbox ADD COLUMN lease_until_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_backup_result_outbox ADD COLUMN next_attempt_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_backup_result_outbox ADD COLUMN attempts bigint NOT NULL DEFAULT 0;
ALTER TABLE regional_backup_result_outbox ADD COLUMN published_at_ms bigint NOT NULL DEFAULT 0;
CREATE INDEX idx_regional_backup_result_publication ON regional_backup_result_outbox(next_attempt_ms,created_at,id) WHERE published_at_ms = 0;
