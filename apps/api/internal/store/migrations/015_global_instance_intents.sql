CREATE TABLE logical_servers (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES organizations(id),
 name text NOT NULL,
 current_revision_id text NOT NULL,
 spec_generation bigint NOT NULL CHECK (spec_generation > 0),
 desired_state text NOT NULL CHECK (desired_state IN ('running','stopped','deleted')),
 intent_version bigint NOT NULL CHECK (intent_version > 0),
 created_at timestamp NOT NULL
);
CREATE INDEX idx_logical_servers_owner ON logical_servers(organization_id);
CREATE TABLE server_revisions (
 id text PRIMARY KEY,
 server_id text NOT NULL REFERENCES logical_servers(id),
 spec_generation bigint NOT NULL CHECK (spec_generation > 0),
 specification text NOT NULL,
 cpu double precision NOT NULL CHECK (cpu > 0),
 memory_mb bigint NOT NULL CHECK (memory_mb > 0),
 created_at timestamp NOT NULL,
 UNIQUE(server_id,spec_generation)
);
CREATE TABLE server_placements (
 server_id text PRIMARY KEY REFERENCES logical_servers(id),
 region_id text NOT NULL CHECK (length(region_id) > 0),
 placement_epoch bigint NOT NULL CHECK (placement_epoch > 0)
);
CREATE TABLE server_operations (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES organizations(id),
 server_id text NOT NULL REFERENCES logical_servers(id),
 revision_id text NOT NULL REFERENCES server_revisions(id),
 kind text NOT NULL,
 status text NOT NULL,
 idempotency_key text NOT NULL,
 request_hash text NOT NULL,
 created_at timestamp NOT NULL,
 UNIQUE(organization_id,kind,idempotency_key)
);
CREATE TABLE server_outbox (
 id text PRIMARY KEY,
 operation_id text NOT NULL UNIQUE REFERENCES server_operations(id),
 region_id text NOT NULL,
 payload text NOT NULL,
 created_at timestamp NOT NULL
);
CREATE INDEX idx_server_outbox_region ON server_outbox(region_id,created_at,id);
