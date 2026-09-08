CREATE TABLE regional_backup_inbox (
 event_id text PRIMARY KEY,
 operation_id text NOT NULL,
 payload_hash text NOT NULL,
 received_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE regional_backup_requests (
 operation_id text PRIMARY KEY,
 backup_id text NOT NULL UNIQUE,
 event_id text NOT NULL REFERENCES regional_backup_inbox(event_id),
 payload_hash text NOT NULL,
 payload text NOT NULL CHECK(octet_length(payload) <= 16384),
 status text NOT NULL DEFAULT 'awaiting_authority' CHECK(status IN ('awaiting_authority','preparing','uploaded','rejected')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_regional_backup_requests_pending ON regional_backup_requests(created_at,operation_id) WHERE status = 'awaiting_authority';
