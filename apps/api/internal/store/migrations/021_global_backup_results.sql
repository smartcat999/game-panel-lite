CREATE TABLE global_backup_results (
 operation_id text PRIMARY KEY REFERENCES server_operations(id),
 event_id text NOT NULL UNIQUE,
 payload_hash text NOT NULL,
 payload text NOT NULL,
 disposition text NOT NULL CHECK(disposition IN ('published','discarded')),
 created_at timestamp NOT NULL
);
