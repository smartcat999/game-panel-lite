CREATE TABLE provider_releases (
    id text PRIMARY KEY,
    game_key text NOT NULL,
    display_name text NOT NULL,
    release_version text NOT NULL,
    manifest jsonb NOT NULL,
    manifest_digest text NOT NULL UNIQUE,
    published_at timestamptz NOT NULL
);
CREATE INDEX provider_releases_catalog_idx ON provider_releases (game_key, release_version, id);

ALTER TABLE managed_instances
    ADD COLUMN game_version text NOT NULL DEFAULT '',
    ADD COLUMN mod_lock jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN telemetry_sequence bigint NOT NULL DEFAULT 0;

CREATE TABLE managed_instance_revisions (
    id text PRIMARY KEY,
    operation_id text NOT NULL UNIQUE,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    provider_release_id text NOT NULL,
    game_version text NOT NULL,
    schema_version integer NOT NULL CHECK (schema_version > 0),
    configuration jsonb NOT NULL,
    mod_lock jsonb NOT NULL,
    apply_behavior text NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE INDEX managed_instance_revisions_instance_idx ON managed_instance_revisions (logical_instance_id, created_at DESC, id);

CREATE TABLE configuration_drafts (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    base_revision_id text NOT NULL,
    schema_version integer NOT NULL CHECK (schema_version > 0),
    values jsonb NOT NULL,
    mod_selections jsonb NOT NULL,
    validation_errors jsonb NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX configuration_drafts_instance_idx ON configuration_drafts (logical_instance_id, updated_at DESC, id);

CREATE TABLE instance_log_entries (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    runtime_attempt_id text,
    stream text NOT NULL,
    message text NOT NULL,
    observed_at timestamptz NOT NULL
);
CREATE INDEX instance_log_entries_query_idx ON instance_log_entries (logical_instance_id, observed_at DESC, id);

CREATE TABLE instance_metric_samples (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    runtime_attempt_id text,
    metric text NOT NULL,
    value double precision NOT NULL,
    unit text NOT NULL,
    source text NOT NULL,
    confidence double precision,
    fresh_until timestamptz,
    sampled_at timestamptz NOT NULL
);
CREATE INDEX instance_metric_samples_query_idx ON instance_metric_samples (logical_instance_id, metric, sampled_at DESC, id);

CREATE TABLE managed_backups (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    region_id text NOT NULL,
    operation_id text NOT NULL,
    provider_release_id text NOT NULL,
    game_version text NOT NULL,
    configuration_revision_id text NOT NULL,
    mod_lock jsonb NOT NULL,
    checksums jsonb NOT NULL,
    object_key text,
    size_bytes bigint CHECK (size_bytes >= 0),
    status text NOT NULL,
    sequence bigint NOT NULL DEFAULT 0,
    idempotency_key text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (workspace_id, idempotency_key)
);
CREATE INDEX managed_backups_workspace_idx ON managed_backups (workspace_id, created_at DESC, id);
CREATE INDEX managed_backups_instance_idx ON managed_backups (logical_instance_id, created_at DESC, id);
