CREATE TABLE regions (
    id text PRIMARY KEY,
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    available boolean NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE plans (
    id text PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE plan_versions (
    id text PRIMARY KEY,
    plan_id text NOT NULL,
    version integer NOT NULL,
    name text NOT NULL,
    price_minor bigint NOT NULL,
    currency text NOT NULL,
    billing_period text NOT NULL,
    memory_megabytes integer NOT NULL,
    cpu_units integer NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (plan_id, version)
);
CREATE INDEX plan_versions_plan_id_idx ON plan_versions (plan_id);

CREATE TABLE plan_version_regions (
    plan_version_id text NOT NULL,
    region_id text NOT NULL,
    PRIMARY KEY (plan_version_id, region_id)
);
CREATE INDEX plan_version_regions_region_id_idx ON plan_version_regions (region_id);

CREATE TABLE logical_instances (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    name text NOT NULL,
    game_key text NOT NULL,
    desired_state text NOT NULL,
    billing_state text NOT NULL,
    deployment_state text NOT NULL,
    stale boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL
);
CREATE INDEX logical_instances_workspace_id_idx ON logical_instances (workspace_id, id);

CREATE TABLE instance_revisions (
    id text PRIMARY KEY,
    logical_instance_id text NOT NULL,
    version integer NOT NULL,
    game_version text NOT NULL,
    configuration jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (logical_instance_id, version)
);
CREATE INDEX instance_revisions_instance_id_idx ON instance_revisions (logical_instance_id, version);

CREATE TABLE placements (
    id text PRIMARY KEY,
    logical_instance_id text NOT NULL,
    region_id text NOT NULL,
    version integer NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (logical_instance_id, version)
);
CREATE INDEX placements_instance_id_idx ON placements (logical_instance_id, version);
CREATE INDEX placements_region_id_idx ON placements (region_id, id);

CREATE TABLE orders (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    plan_version_id text NOT NULL,
    status text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE INDEX orders_workspace_id_idx ON orders (workspace_id, id);
CREATE INDEX orders_instance_id_idx ON orders (logical_instance_id, id);

CREATE TABLE payments (
    id text PRIMARY KEY,
    order_id text NOT NULL,
    provider_notification_id text NOT NULL UNIQUE,
    verified_at timestamptz NOT NULL
);
CREATE INDEX payments_order_id_idx ON payments (order_id, id);

CREATE TABLE entitlements (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    plan_version_id text NOT NULL,
    effective_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    active boolean NOT NULL
);
CREATE INDEX entitlements_workspace_id_idx ON entitlements (workspace_id, id);
CREATE INDEX entitlements_instance_id_idx ON entitlements (logical_instance_id, expires_at);

CREATE TABLE command_results (
    idempotency_key text PRIMARY KEY,
    command_id text NOT NULL,
    result jsonb NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE global_outbox (
    id text PRIMARY KEY,
    message_type text NOT NULL,
    schema_version integer NOT NULL,
    idempotency_key text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    published_at timestamptz,
    UNIQUE (message_type, idempotency_key)
);
CREATE INDEX global_outbox_unpublished_idx ON global_outbox (published_at, created_at, id);

CREATE TABLE global_inbox (
    message_id text PRIMARY KEY,
    message_type text NOT NULL,
    received_at timestamptz NOT NULL,
    handled_at timestamptz
);
CREATE INDEX global_inbox_pending_idx ON global_inbox (handled_at, received_at, message_id);
