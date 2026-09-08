CREATE TABLE regional_node_sessions (
 node_id text PRIMARY KEY REFERENCES regional_nodes(id),
 epoch bigint NOT NULL CHECK(epoch > 0),
 sequence bigint NOT NULL DEFAULT 0 CHECK(sequence >= 0),
 architecture text NOT NULL DEFAULT '',
 runtime_ready boolean NOT NULL DEFAULT false,
 last_seen_ms bigint NOT NULL DEFAULT 0 CHECK(last_seen_ms >= 0)
);
