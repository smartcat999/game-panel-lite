CREATE TABLE regional_delivery_nodes (
    id text PRIMARY KEY,
    region_id text NOT NULL,
    state text NOT NULL,
    cpu_capacity_milli bigint NOT NULL CHECK (cpu_capacity_milli > 0),
    memory_capacity_mib bigint NOT NULL CHECK (memory_capacity_mib > 0),
    disk_capacity_gib bigint NOT NULL CHECK (disk_capacity_gib > 0),
    reserved_cpu_milli bigint NOT NULL DEFAULT 0 CHECK (reserved_cpu_milli >= 0 AND reserved_cpu_milli <= cpu_capacity_milli),
    reserved_memory_mib bigint NOT NULL DEFAULT 0 CHECK (reserved_memory_mib >= 0 AND reserved_memory_mib <= memory_capacity_mib),
    reserved_disk_gib bigint NOT NULL DEFAULT 0 CHECK (reserved_disk_gib >= 0 AND reserved_disk_gib <= disk_capacity_gib),
    lease_until timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX regional_delivery_nodes_schedulable_idx ON regional_delivery_nodes (region_id, state, lease_until, id);

CREATE TABLE endpoint_pools (
    id text PRIMARY KEY,
    region_id text NOT NULL,
    delivery_mode text NOT NULL,
    address text NOT NULL,
    port_start integer,
    port_end integer,
    stability text NOT NULL,
    active boolean NOT NULL,
    CHECK ((delivery_mode = 'dedicated-ip' AND port_start IS NULL AND port_end IS NULL) OR
           (delivery_mode IN ('gateway', 'node-direct') AND port_start BETWEEN 1 AND 65535 AND port_end BETWEEN port_start AND 65535))
);
CREATE INDEX endpoint_pools_region_idx ON endpoint_pools (region_id, active, delivery_mode, id);

CREATE TABLE regional_delivery_states (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL UNIQUE,
    region_id text NOT NULL,
    placement_version bigint NOT NULL CHECK (placement_version > 0),
    instance_revision_id text NOT NULL,
    operation_id text NOT NULL,
    desired_state text NOT NULL,
    provider_release_id text NOT NULL,
    cpu_milli bigint NOT NULL CHECK (cpu_milli > 0),
    memory_mib bigint NOT NULL CHECK (memory_mib > 0),
    disk_gib bigint NOT NULL CHECK (disk_gib > 0),
    configuration jsonb NOT NULL,
    listener_requirements jsonb NOT NULL,
    phase text NOT NULL,
    node_id text,
    fencing_token bigint NOT NULL DEFAULT 0,
    reconcile_owner text,
    reconcile_lease_until timestamptz,
    assignment_owner text,
    assignment_lease_until timestamptz,
    assignment_attempts integer NOT NULL DEFAULT 0,
    observation_sequence bigint NOT NULL DEFAULT 0,
    failure_code text,
    residual_cleanup_required boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL
);
CREATE INDEX regional_delivery_states_reconcile_idx ON regional_delivery_states (phase, reconcile_lease_until, updated_at, id);
CREATE INDEX regional_delivery_states_node_idx ON regional_delivery_states (node_id, phase, id);
CREATE SEQUENCE regional_delivery_fencing_token_seq;

CREATE TABLE endpoint_allocations (
    id text PRIMARY KEY,
    regional_delivery_id text NOT NULL,
    logical_instance_id text NOT NULL,
    pool_id text NOT NULL,
    listener_name text NOT NULL,
    purpose text NOT NULL,
    address text NOT NULL,
    port integer,
    transports jsonb NOT NULL,
    stability text NOT NULL,
    display_address text NOT NULL,
    is_primary boolean NOT NULL,
    allocation_key text NOT NULL,
    active boolean NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX endpoint_allocations_active_key_idx ON endpoint_allocations (allocation_key) WHERE active = true;
CREATE UNIQUE INDEX endpoint_allocations_active_listener_idx ON endpoint_allocations (regional_delivery_id, listener_name) WHERE active = true;
CREATE INDEX endpoint_allocations_deployment_idx ON endpoint_allocations (regional_delivery_id, active, id);

ALTER TABLE regional_outbox
    ADD COLUMN attempt_count integer NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at timestamptz NOT NULL DEFAULT '-infinity',
    ADD COLUMN claimed_until timestamptz,
    ADD COLUMN last_error text;
