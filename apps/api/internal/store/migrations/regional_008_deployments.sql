CREATE TABLE regional_deployments (
 id text PRIMARY KEY,
 organization_id text NOT NULL,
 server_id text NOT NULL,
 placement_epoch bigint NOT NULL CHECK(placement_epoch > 0),
 revision_id text NOT NULL,
 revision_operation_id text NOT NULL REFERENCES regional_revision_tasks(operation_id),
 spec_generation bigint NOT NULL CHECK(spec_generation > 0),
 intent_version bigint NOT NULL CHECK(intent_version > 0),
 desired_state text NOT NULL CHECK(desired_state IN ('running','stopped')),
 status text NOT NULL DEFAULT 'awaiting_authority' CHECK(status IN ('awaiting_authority')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(server_id,placement_epoch)
);
CREATE INDEX idx_regional_deployments_pending ON regional_deployments(created_at,id) WHERE status = 'awaiting_authority';
