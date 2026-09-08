ALTER TABLE server_outbox ADD COLUMN lease_token text NOT NULL DEFAULT '';
ALTER TABLE server_outbox ADD COLUMN lease_until_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE server_outbox ADD COLUMN next_attempt_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE server_outbox ADD COLUMN published_at_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE server_outbox ADD COLUMN attempts bigint NOT NULL DEFAULT 0;
CREATE INDEX idx_server_outbox_pending ON server_outbox(region_id,next_attempt_ms,created_at,id) WHERE published_at_ms = 0;
