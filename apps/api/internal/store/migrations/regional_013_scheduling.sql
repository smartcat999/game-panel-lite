ALTER TABLE regional_deployments
 ADD COLUMN scheduling_status text NOT NULL DEFAULT 'pending' CHECK(scheduling_status IN ('pending','reserved','rejected')),
 ADD COLUMN scheduling_token text NOT NULL DEFAULT '',
 ADD COLUMN scheduling_until_ms bigint NOT NULL DEFAULT 0,
 ADD COLUMN scheduling_next_ms bigint NOT NULL DEFAULT 0,
 ADD COLUMN scheduling_attempts bigint NOT NULL DEFAULT 0;
CREATE INDEX idx_regional_deployments_scheduling ON regional_deployments(scheduling_next_ms,created_at,id)
 WHERE scheduling_status = 'pending' AND desired_state = 'running';
