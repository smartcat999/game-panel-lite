-- Initial PostgreSQL schema, captured from the pre-migration Store on PostgreSQL 16.
-- Immutable: append a new numbered migration for future schema changes.
CREATE TABLE activity_events (
    id text NOT NULL,
    instance_id text,
    type text,
    message text,
    payload_json text,
    created_at timestamp with time zone
);

CREATE TABLE admin_accounts (
    id text NOT NULL,
    username text,
    role text DEFAULT 'admin'::text,
    password_hash text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE backups (
    config_version bigint,
    id text NOT NULL,
    instance_id text,
    game_key text,
    provider_key text,
    file_name text,
    world_name text,
    size_bytes bigint,
    type text,
    created_at timestamp with time zone
);

CREATE TABLE compute_nodes (
    id text NOT NULL,
    name text,
    host text,
    port bigint,
    token text,
    public_ip text,
    region text,
    status text,
    is_local boolean,
    cpu_cores bigint,
    cpu_usage_percent numeric,
    memory_total_mb bigint,
    memory_used_mb bigint,
    disk_total_gb bigint,
    disk_used_gb bigint,
    docker_version text,
    agent_version text,
    os_info text,
    ping_latency_ms bigint,
    running_count bigint,
    last_heartbeat timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE config_presets (
    id text NOT NULL,
    name text,
    game_key text,
    provider_key text,
    version text,
    config_payload_json text,
    cpu_limit_cores numeric,
    memory_limit_mb bigint,
    mod_pack_id text,
    mod_ids_json text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE game_servers (
    id text NOT NULL,
    organization_id text,
    node_id text,
    name text,
    game_key text,
    provider_key text,
    spec text,
    status text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE game_update_jobs (
    id text NOT NULL,
    instance_id text,
    provider_key text,
    operation text,
    status text,
    stage text,
    progress bigint,
    installed_build_id text,
    latest_build_id text,
    start_after_update boolean,
    was_running boolean,
    error text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    checked_at timestamp with time zone,
    completed_at timestamp with time zone
);

CREATE TABLE mod_files (
    id text NOT NULL,
    instance_id text,
    game_key text,
    provider_key text,
    file_name text,
    source text,
    workshop_id text,
    mod_name text,
    title text,
    mod_version text,
    t_mod_version text,
    creator_steam_id text,
    preview_url text,
    description text,
    content_hash text,
    tags_json text,
    subscriptions bigint,
    favorited bigint,
    views bigint,
    updated_at_steam bigint,
    size_bytes bigint,
    enabled boolean,
    dependencies_json text,
    created_at timestamp with time zone
);

CREATE TABLE mod_packs (
    id text NOT NULL,
    name text,
    description text,
    mod_ids_json text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE node_tasks (
    id text NOT NULL,
    node_id text,
    server_id text,
    action text,
    payload text,
    image text,
    env text,
    ports text,
    status text,
    error text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE organization_members (
    id text NOT NULL,
    organization_id text,
    user_id text,
    role text,
    created_at timestamp with time zone
);

CREATE TABLE organizations (
    id text NOT NULL,
    name text,
    slug text,
    plan text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE server_shares (
    token text NOT NULL,
    instance_id text,
    include_password boolean,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE sessions (
    id text NOT NULL,
    account_id text,
    token_hash text,
    expires_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE settings (
    key text NOT NULL,
    value text
);

CREATE TABLE tenant_quota (
    organization_id text NOT NULL,
    max_servers bigint,
    max_cpu_cores numeric,
    max_memory_mb bigint,
    max_storage_gb bigint
);

CREATE TABLE workload_assignments (
    id text NOT NULL,
    uid text,
    server_id text,
    node_id text,
    generation bigint,
    desired_state text,
    spec text,
    deletion_timestamp timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE workload_observations (
    id text NOT NULL,
    assignment_uid text,
    server_id text,
    node_id text,
    observed_generation bigint,
    runtime_id text,
    actual_state text,
    conditions text,
    last_error text,
    observed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE world_regeneration_jobs (
    id text NOT NULL,
    instance_id text,
    provider_key text,
    status text,
    stage text,
    progress bigint,
    backup_id text,
    start_after boolean,
    was_running boolean,
    error text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    completed_at timestamp with time zone
);

CREATE TABLE worlds (
    id text NOT NULL,
    instance_id text,
    provider_key text,
    name text,
    file_name text,
    size_bytes bigint,
    source text,
    config_payload_json text,
    active_instance_id text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY activity_events
    ADD CONSTRAINT activity_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY admin_accounts
    ADD CONSTRAINT admin_accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY backups
    ADD CONSTRAINT backups_pkey PRIMARY KEY (id);

ALTER TABLE ONLY compute_nodes
    ADD CONSTRAINT compute_nodes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY config_presets
    ADD CONSTRAINT config_presets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY game_servers
    ADD CONSTRAINT game_servers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY game_update_jobs
    ADD CONSTRAINT game_update_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY mod_files
    ADD CONSTRAINT mod_files_pkey PRIMARY KEY (id);

ALTER TABLE ONLY mod_packs
    ADD CONSTRAINT mod_packs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY node_tasks
    ADD CONSTRAINT node_tasks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY organization_members
    ADD CONSTRAINT organization_members_pkey PRIMARY KEY (id);

ALTER TABLE ONLY organizations
    ADD CONSTRAINT organizations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY server_shares
    ADD CONSTRAINT server_shares_pkey PRIMARY KEY (token);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY settings
    ADD CONSTRAINT settings_pkey PRIMARY KEY (key);

ALTER TABLE ONLY tenant_quota
    ADD CONSTRAINT tenant_quota_pkey PRIMARY KEY (organization_id);

ALTER TABLE ONLY workload_assignments
    ADD CONSTRAINT workload_assignments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workload_observations
    ADD CONSTRAINT workload_observations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY world_regeneration_jobs
    ADD CONSTRAINT world_regeneration_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY worlds
    ADD CONSTRAINT worlds_pkey PRIMARY KEY (id);

CREATE INDEX idx_activity_events_instance_id ON activity_events USING btree (instance_id);

CREATE UNIQUE INDEX idx_admin_accounts_username ON admin_accounts USING btree (username);

CREATE INDEX idx_backups_instance_id ON backups USING btree (instance_id);

CREATE INDEX idx_config_presets_game_key ON config_presets USING btree (game_key);

CREATE INDEX idx_config_presets_mod_pack_id ON config_presets USING btree (mod_pack_id);

CREATE INDEX idx_config_presets_provider_key ON config_presets USING btree (provider_key);

CREATE INDEX idx_game_servers_game_key ON game_servers USING btree (game_key);

CREATE INDEX idx_game_servers_node_id ON game_servers USING btree (node_id);

CREATE INDEX idx_game_servers_organization_id ON game_servers USING btree (organization_id);

CREATE INDEX idx_game_servers_provider_key ON game_servers USING btree (provider_key);

CREATE INDEX idx_game_update_jobs_instance_id ON game_update_jobs USING btree (instance_id);

CREATE INDEX idx_game_update_jobs_operation ON game_update_jobs USING btree (operation);

CREATE INDEX idx_game_update_jobs_provider_key ON game_update_jobs USING btree (provider_key);

CREATE INDEX idx_game_update_jobs_status ON game_update_jobs USING btree (status);

CREATE INDEX idx_mod_files_content_hash ON mod_files USING btree (content_hash);

CREATE INDEX idx_mod_files_game_key ON mod_files USING btree (game_key);

CREATE INDEX idx_mod_files_instance_id ON mod_files USING btree (instance_id);

CREATE INDEX idx_mod_files_mod_name ON mod_files USING btree (mod_name);

CREATE INDEX idx_mod_files_provider_key ON mod_files USING btree (provider_key);

CREATE INDEX idx_mod_files_source ON mod_files USING btree (source);

CREATE INDEX idx_mod_files_workshop_id ON mod_files USING btree (workshop_id);

CREATE INDEX idx_node_tasks_node_id ON node_tasks USING btree (node_id);

CREATE INDEX idx_node_tasks_server_id ON node_tasks USING btree (server_id);

CREATE INDEX idx_node_tasks_status ON node_tasks USING btree (status);

CREATE UNIQUE INDEX idx_org_user ON organization_members USING btree (organization_id, user_id);

CREATE UNIQUE INDEX idx_organizations_slug ON organizations USING btree (slug);

CREATE UNIQUE INDEX idx_server_shares_instance_id ON server_shares USING btree (instance_id);

CREATE INDEX idx_sessions_account_id ON sessions USING btree (account_id);

CREATE INDEX idx_sessions_expires_at ON sessions USING btree (expires_at);

CREATE UNIQUE INDEX idx_sessions_token_hash ON sessions USING btree (token_hash);

CREATE INDEX idx_workload_assignments_node_id ON workload_assignments USING btree (node_id);

CREATE UNIQUE INDEX idx_workload_assignments_server_id ON workload_assignments USING btree (server_id);

CREATE UNIQUE INDEX idx_workload_assignments_uid ON workload_assignments USING btree (uid);

CREATE UNIQUE INDEX idx_workload_observations_assignment_uid ON workload_observations USING btree (assignment_uid);

CREATE INDEX idx_workload_observations_node_id ON workload_observations USING btree (node_id);

CREATE INDEX idx_workload_observations_server_id ON workload_observations USING btree (server_id);

CREATE INDEX idx_world_regeneration_jobs_instance_id ON world_regeneration_jobs USING btree (instance_id);

CREATE INDEX idx_world_regeneration_jobs_provider_key ON world_regeneration_jobs USING btree (provider_key);

CREATE INDEX idx_world_regeneration_jobs_status ON world_regeneration_jobs USING btree (status);

CREATE INDEX idx_worlds_instance_id ON worlds USING btree (instance_id);

CREATE INDEX idx_worlds_provider_key ON worlds USING btree (provider_key);
