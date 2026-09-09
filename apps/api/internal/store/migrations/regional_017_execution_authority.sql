ALTER TABLE regional_node_tasks DROP CONSTRAINT regional_node_tasks_status_check;
ALTER TABLE regional_node_tasks ADD CONSTRAINT regional_node_tasks_status_check
 CHECK(status IN ('awaiting_authority','active','succeeded','superseded'));

CREATE TABLE regional_execution_leases (
 server_id text PRIMARY KEY,
 task_id text NOT NULL UNIQUE REFERENCES regional_node_tasks(id),
 node_id text NOT NULL REFERENCES regional_nodes(id),
 session_epoch bigint NOT NULL CHECK(session_epoch > 0),
 generation bigint NOT NULL CHECK(generation > 0),
 holder_id text NOT NULL DEFAULT '',
 fence bigint NOT NULL DEFAULT 0 CHECK(fence >= 0),
 granted_at_ms bigint NOT NULL DEFAULT 0 CHECK(granted_at_ms >= 0),
 expires_at_ms bigint NOT NULL DEFAULT 0 CHECK(expires_at_ms >= 0)
);
CREATE INDEX idx_regional_execution_lease_expiry ON regional_execution_leases(expires_at_ms,server_id);

CREATE TABLE regional_workload_observations (
 task_id text PRIMARY KEY REFERENCES regional_node_tasks(id),
 server_id text NOT NULL,
 node_id text NOT NULL,
 generation bigint NOT NULL CHECK(generation > 0),
 holder_id text NOT NULL,
 fence bigint NOT NULL CHECK(fence > 0),
 input_token text NOT NULL DEFAULT '',
 observation_token text NOT NULL,
 payload_sha256 text NOT NULL CHECK(length(payload_sha256) = 64),
 payload jsonb NOT NULL CHECK(octet_length(payload::text) <= 65536),
 observed_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_regional_workload_observation_server ON regional_workload_observations(server_id,task_id);
