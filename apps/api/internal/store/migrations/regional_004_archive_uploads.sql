CREATE TABLE regional_archive_uploads (
 id text PRIMARY KEY,
 operation_id text NOT NULL UNIQUE,
 request_event_id text NOT NULL,
 storage_id text NOT NULL,
 object_key text NOT NULL,
 plan text NOT NULL CHECK (octet_length(plan) <= 16384),
 status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','uploaded','invalid')),
 receipt text NOT NULL DEFAULT '' CHECK (octet_length(receipt) <= 16384),
 lease_token text NOT NULL DEFAULT '',
 lease_until_ms bigint NOT NULL DEFAULT 0,
 next_attempt_ms bigint NOT NULL DEFAULT 0,
 attempts bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(storage_id,object_key)
);
CREATE INDEX idx_regional_archive_uploads_ready ON regional_archive_uploads(next_attempt_ms,created_at,id) WHERE status = 'pending';
CREATE TABLE regional_backup_result_outbox (
 id text PRIMARY KEY,
 upload_id text NOT NULL UNIQUE REFERENCES regional_archive_uploads(id),
 operation_id text NOT NULL,
 event_type text NOT NULL,
 payload text NOT NULL CHECK (octet_length(payload) <= 32768),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
