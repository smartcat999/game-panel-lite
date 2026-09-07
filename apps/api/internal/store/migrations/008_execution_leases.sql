-- Keep lease tombstones after assignment deletion to preserve fencing history.
CREATE TABLE workload_execution_leases (
    server_id text PRIMARY KEY,
    assignment_uid text NOT NULL DEFAULT '',
    node_id text NOT NULL DEFAULT '',
    generation bigint NOT NULL DEFAULT 0,
    holder_id text NOT NULL DEFAULT '',
    fence bigint NOT NULL DEFAULT 0,
    expires_at_ms bigint NOT NULL DEFAULT 0
);
