CREATE TABLE global_deployment_statuses (
 server_id text PRIMARY KEY REFERENCES logical_servers(id),
 organization_id text NOT NULL REFERENCES organizations(id),
 region_id text NOT NULL REFERENCES global_regions(id),
 operation_id text NOT NULL REFERENCES server_operations(id),
 revision_id text NOT NULL REFERENCES server_revisions(id),
 task_id text NOT NULL,
 node_id text NOT NULL,
 placement_epoch bigint NOT NULL CHECK(placement_epoch > 0),
 spec_generation bigint NOT NULL CHECK(spec_generation > 0),
 intent_version bigint NOT NULL CHECK(intent_version > 0),
 fence bigint NOT NULL CHECK(fence > 0),
 actual_state text NOT NULL CHECK(actual_state IN ('running','stopped','missing','unknown')),
 outcome text NOT NULL CHECK(outcome IN ('succeeded','failed')),
 runtime_id text NOT NULL DEFAULT '',
 observed_at_ms bigint NOT NULL CHECK(observed_at_ms > 0),
 event_id text NOT NULL UNIQUE,
 payload_hash text NOT NULL CHECK(length(payload_hash) = 64),
 received_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_global_deployment_status_owner ON global_deployment_statuses(organization_id,server_id);
CREATE INDEX idx_global_deployment_status_region ON global_deployment_statuses(region_id,server_id);
