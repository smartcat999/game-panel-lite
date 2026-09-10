CREATE TABLE backup_requests (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    region_id text NOT NULL,
    kind text NOT NULL,
    status text NOT NULL,
    sequence bigint NOT NULL DEFAULT 0,
    object_key text,
    size_bytes bigint,
    checksum text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX backup_requests_workspace_idx ON backup_requests (workspace_id, created_at, id);
CREATE INDEX backup_requests_instance_idx ON backup_requests (logical_instance_id, created_at, id);

CREATE TABLE backup_command_results (
    idempotency_key text PRIMARY KEY,
    backup_request_id text NOT NULL,
    created_at timestamptz NOT NULL
);
