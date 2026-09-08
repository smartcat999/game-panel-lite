CREATE TABLE global_server_entitlements (
 server_id text PRIMARY KEY REFERENCES logical_servers(id),
 organization_id text NOT NULL,
 version bigint NOT NULL CHECK(version > 0),
 cpu double precision NOT NULL CHECK(cpu > 0 AND cpu <= 1.7976931348623157e308),
 memory_mb bigint NOT NULL CHECK(memory_mb > 0),
 starts_at_ms bigint NOT NULL CHECK(starts_at_ms >= 0),
 ends_at_ms bigint NOT NULL CHECK(ends_at_ms > starts_at_ms),
 status text NOT NULL CHECK(status IN ('active','suspended','revoked')),
 source_kind text NOT NULL CHECK(source_kind = 'operator'),
 source_id text NOT NULL UNIQUE
);
CREATE TABLE global_entitlement_changes (
 source_id text PRIMARY KEY,
 source_kind text NOT NULL CHECK(source_kind = 'operator'),
 server_id text NOT NULL REFERENCES logical_servers(id),
 organization_id text NOT NULL,
 version bigint NOT NULL CHECK(version > 0),
 cpu double precision NOT NULL CHECK(cpu > 0 AND cpu <= 1.7976931348623157e308),
 memory_mb bigint NOT NULL CHECK(memory_mb > 0),
 starts_at_ms bigint NOT NULL CHECK(starts_at_ms >= 0),
 ends_at_ms bigint NOT NULL CHECK(ends_at_ms > starts_at_ms),
 status text NOT NULL CHECK(status IN ('active','suspended','revoked')),
 actor_id text NOT NULL,
 request_id text NOT NULL,
 request_hash text NOT NULL,
 reason text NOT NULL,
 UNIQUE(actor_id,request_id),
 UNIQUE(server_id,version)
);
