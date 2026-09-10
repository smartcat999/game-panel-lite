CREATE TABLE work_assignments (
    id text PRIMARY KEY,
    regional_task_id text NOT NULL,
    regional_deployment_id text NOT NULL,
    node_id text NOT NULL,
    action text NOT NULL,
    payload jsonb NOT NULL,
    fencing_token bigint NOT NULL,
    status text NOT NULL,
    claimed_by text,
    claim_lease_until timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    completed_at timestamptz
);
CREATE INDEX work_assignments_poll_idx ON work_assignments (node_id, status, claim_lease_until, created_at, id);
CREATE INDEX work_assignments_deployment_idx ON work_assignments (regional_deployment_id, fencing_token, id);

CREATE TABLE region_backup_results (
    backup_request_id text PRIMARY KEY,
    regional_task_id text NOT NULL,
    logical_instance_id text NOT NULL,
    sequence bigint NOT NULL,
    status text NOT NULL,
    object_key text,
    size_bytes bigint,
    checksum text,
    observed_at timestamptz NOT NULL
);
CREATE INDEX region_backup_results_instance_idx ON region_backup_results (logical_instance_id, observed_at, backup_request_id);
