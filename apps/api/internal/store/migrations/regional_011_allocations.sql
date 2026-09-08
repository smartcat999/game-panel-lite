CREATE TABLE regional_allocations (
 id text PRIMARY KEY,
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
 cpu double precision NOT NULL CHECK(cpu > 0 AND cpu < 'Infinity'::double precision),
 memory_mb bigint NOT NULL CHECK(memory_mb > 0),
 status text NOT NULL CHECK(status IN ('reserved','released')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX idx_regional_allocation_active ON regional_allocations(deployment_id) WHERE status = 'reserved';
CREATE INDEX idx_regional_allocation_node ON regional_allocations(node_id) WHERE status = 'reserved';
