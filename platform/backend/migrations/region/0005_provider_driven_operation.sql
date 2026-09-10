ALTER TABLE regional_delivery_states
    ADD COLUMN game_version text NOT NULL DEFAULT '',
    ADD COLUMN apply_behavior text NOT NULL DEFAULT 'recreate-required',
    ADD COLUMN mod_lock jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN telemetry_sequence bigint NOT NULL DEFAULT 0;

CREATE TABLE regional_instance_tasks (
    id text PRIMARY KEY,
    message_id text NOT NULL UNIQUE,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    region_id text NOT NULL,
    operation_id text NOT NULL,
    kind text NOT NULL,
    payload jsonb NOT NULL,
    status text NOT NULL,
    claim_owner text,
    claim_until timestamptz,
    attempt_count integer NOT NULL DEFAULT 0,
    fencing_token bigint NOT NULL,
    failure_code text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX regional_instance_tasks_claim_idx ON regional_instance_tasks (status, claim_until, created_at, id);
CREATE INDEX regional_instance_tasks_instance_idx ON regional_instance_tasks (logical_instance_id, created_at DESC, id);
