CREATE TABLE regional_identity (
 id integer PRIMARY KEY CHECK (id = 1),
 region_id text NOT NULL CHECK (length(region_id) > 0)
);
CREATE TABLE regional_inbox (
 event_id text PRIMARY KEY,
 operation_id text NOT NULL,
 payload_hash text NOT NULL,
 received_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_regional_inbox_operation ON regional_inbox(operation_id);
CREATE TABLE regional_revision_tasks (
 operation_id text PRIMARY KEY,
 event_id text NOT NULL REFERENCES regional_inbox(event_id),
 payload_hash text NOT NULL,
 payload text NOT NULL,
 status text NOT NULL DEFAULT 'awaiting_revision' CHECK(status IN ('awaiting_revision','materialized','rejected')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_regional_revision_tasks_pending ON regional_revision_tasks(created_at,operation_id) WHERE status = 'awaiting_revision';
