CREATE TABLE regional_deployment_status_outbox (
 id text PRIMARY KEY,
 server_id text NOT NULL,
 task_id text NOT NULL REFERENCES regional_node_tasks(id),
 fence bigint NOT NULL CHECK(fence > 0),
 event_type text NOT NULL CHECK(event_type = 'deployment.status.observed'),
 payload text NOT NULL,
 lease_token text NOT NULL DEFAULT '',
 lease_until_ms bigint NOT NULL DEFAULT 0,
 next_attempt_ms bigint NOT NULL DEFAULT 0,
 attempts bigint NOT NULL DEFAULT 0,
 published_at_ms bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(task_id,fence)
);
CREATE INDEX idx_regional_deployment_status_publication ON regional_deployment_status_outbox(next_attempt_ms,created_at,id) WHERE published_at_ms = 0;
