CREATE TABLE regional_inbox (
    message_id text PRIMARY KEY,
    message_type text NOT NULL,
    received_at timestamptz NOT NULL,
    handled_at timestamptz
);
CREATE INDEX regional_inbox_pending_idx ON regional_inbox (handled_at, received_at, message_id);

CREATE TABLE regional_deployments (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL UNIQUE,
    region_id text NOT NULL,
    placement_version integer NOT NULL,
    instance_revision_id text NOT NULL,
    desired_state text NOT NULL,
    observed_state text NOT NULL,
    observation_sequence bigint NOT NULL DEFAULT 0,
    game_key text NOT NULL,
    cpu_units integer NOT NULL,
    memory_megabytes integer NOT NULL,
    node_id text,
    unschedulable_reason text,
    updated_at timestamptz NOT NULL
);
CREATE INDEX regional_deployments_region_idx ON regional_deployments (region_id, id);
CREATE INDEX regional_deployments_node_idx ON regional_deployments (node_id, id);

CREATE TABLE nodes (
    id text PRIMARY KEY,
    region_id text NOT NULL,
    name text NOT NULL,
    state text NOT NULL,
    games jsonb NOT NULL,
    cpu_capacity integer NOT NULL,
    memory_capacity_mb integer NOT NULL,
    reserved_cpu integer NOT NULL DEFAULT 0,
    reserved_memory_mb integer NOT NULL DEFAULT 0,
    lease_until timestamptz NOT NULL,
    last_heartbeat_at timestamptz NOT NULL,
    CHECK (reserved_cpu >= 0 AND reserved_cpu <= cpu_capacity),
    CHECK (reserved_memory_mb >= 0 AND reserved_memory_mb <= memory_capacity_mb)
);
CREATE INDEX nodes_region_state_idx ON nodes (region_id, state, id);
CREATE INDEX nodes_lease_idx ON nodes (lease_until, id);

CREATE TABLE reservations (
    id text PRIMARY KEY,
    regional_deployment_id text NOT NULL,
    node_id text NOT NULL,
    cpu_units integer NOT NULL,
    memory_megabytes integer NOT NULL,
    fencing_token bigint NOT NULL,
    active boolean NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX reservations_active_deployment_idx ON reservations (regional_deployment_id) WHERE active = true;
CREATE INDEX reservations_node_idx ON reservations (node_id, active, id);
CREATE SEQUENCE reservation_fencing_token_seq;

CREATE TABLE regional_tasks (
    id text PRIMARY KEY,
    regional_deployment_id text NOT NULL,
    kind text NOT NULL,
    status text NOT NULL,
    attempts integer NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE INDEX regional_tasks_status_idx ON regional_tasks (status, created_at, id);
CREATE INDEX regional_tasks_deployment_idx ON regional_tasks (regional_deployment_id, id);

CREATE TABLE regional_observations (
    message_id text PRIMARY KEY,
    regional_deployment_id text NOT NULL,
    sequence bigint NOT NULL,
    observed_state text NOT NULL,
    observed_at timestamptz NOT NULL
);
CREATE INDEX regional_observations_deployment_idx ON regional_observations (regional_deployment_id, sequence);

CREATE TABLE regional_outbox (
    id text PRIMARY KEY,
    message_type text NOT NULL,
    schema_version integer NOT NULL,
    idempotency_key text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    published_at timestamptz,
    UNIQUE (message_type, idempotency_key)
);
CREATE INDEX regional_outbox_unpublished_idx ON regional_outbox (published_at, created_at, id);

CREATE TABLE region_audit_records (
    id text PRIMARY KEY,
    actor_user_id text NOT NULL,
    region_id text NOT NULL,
    regional_deployment_id text NOT NULL,
    previous_node_id text,
    requested_node_id text NOT NULL,
    reason text NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE INDEX region_audit_records_region_idx ON region_audit_records (region_id, created_at, id);
