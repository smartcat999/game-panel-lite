CREATE TABLE regional_node_tasks (
 id text PRIMARY KEY,
 allocation_id text NOT NULL UNIQUE REFERENCES regional_allocations(id),
 region_id text NOT NULL,
 organization_id text NOT NULL,
 deployment_id text NOT NULL REFERENCES regional_deployments(id),
 server_id text NOT NULL,
 placement_epoch bigint NOT NULL CHECK(placement_epoch > 0),
 revision_id text NOT NULL,
 spec_generation bigint NOT NULL CHECK(spec_generation > 0),
 intent_version bigint NOT NULL CHECK(intent_version > 0),
 node_id text NOT NULL REFERENCES regional_nodes(id),
 node_version bigint NOT NULL CHECK(node_version > 0),
 session_epoch bigint NOT NULL CHECK(session_epoch > 0),
 kind text NOT NULL CHECK(kind = 'run'),
 status text NOT NULL CHECK(status IN ('awaiting_authority','superseded')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_regional_node_tasks_pending ON regional_node_tasks(node_id,status,id);

-- Reuse normal scheduling recovery to verify old receipts before staging tasks.
-- Do not fabricate authority or infer a workload from historical allocation rows.
UPDATE regional_deployments SET scheduling_status='pending', scheduling_token='',
 scheduling_until_ms=0, scheduling_next_ms=0
 WHERE scheduling_status='reserved' AND desired_state='running';
