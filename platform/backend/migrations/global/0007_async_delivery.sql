CREATE TABLE managed_instances (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    region_id text NOT NULL,
    name text NOT NULL,
    provider_release_id text NOT NULL,
    quote_id text NOT NULL UNIQUE,
    instance_revision_id text NOT NULL,
    placement_version bigint NOT NULL CHECK (placement_version > 0),
    cpu_milli bigint NOT NULL CHECK (cpu_milli > 0),
    memory_mib bigint NOT NULL CHECK (memory_mib > 0),
    disk_gib bigint NOT NULL CHECK (disk_gib > 0),
    configuration jsonb NOT NULL,
    listener_requirements jsonb NOT NULL,
    desired_state text NOT NULL,
    observed_state text NOT NULL,
    observation_sequence bigint NOT NULL DEFAULT 0,
    endpoint_bindings jsonb NOT NULL DEFAULT '[]'::jsonb,
    latest_operation_id text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX managed_instances_workspace_idx ON managed_instances (workspace_id, id);
CREATE INDEX managed_instances_region_idx ON managed_instances (region_id, id);

CREATE TABLE operations (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    kind text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    idempotency_key text NOT NULL,
    status text NOT NULL,
    steps jsonb NOT NULL,
    failure_code text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (workspace_id, idempotency_key)
);
CREATE INDEX operations_workspace_idx ON operations (workspace_id, created_at DESC, id);
CREATE INDEX operations_resource_idx ON operations (resource_type, resource_id, created_at DESC);

ALTER TABLE global_outbox
    ADD COLUMN attempt_count integer NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at timestamptz NOT NULL DEFAULT '-infinity',
    ADD COLUMN claimed_until timestamptz,
    ADD COLUMN last_error text;
