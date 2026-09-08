CREATE TABLE global_backup_tasks (
 id text PRIMARY KEY,
 operation_id text NOT NULL UNIQUE REFERENCES server_operations(id),
 organization_id text NOT NULL REFERENCES organizations(id),
 server_id text NOT NULL REFERENCES logical_servers(id),
 region_id text NOT NULL,
 status text NOT NULL CHECK(status IN ('requested','running','succeeded','failed','cancelled')),
 command text NOT NULL,
 created_at timestamp NOT NULL
);
CREATE INDEX idx_global_backup_tasks_owner ON global_backup_tasks(organization_id,created_at,id);
CREATE TABLE backup_request_outbox (
 id text PRIMARY KEY,
 operation_id text NOT NULL UNIQUE REFERENCES server_operations(id),
 region_id text NOT NULL,
 payload text NOT NULL,
 created_at timestamp NOT NULL,
 lease_token text NOT NULL DEFAULT '',
 lease_until_ms bigint NOT NULL DEFAULT 0,
 next_attempt_ms bigint NOT NULL DEFAULT 0,
 attempts bigint NOT NULL DEFAULT 0,
 published_at_ms bigint NOT NULL DEFAULT 0
);
CREATE INDEX idx_backup_request_outbox_pending ON backup_request_outbox(region_id,next_attempt_ms,created_at,id) WHERE published_at_ms = 0;
