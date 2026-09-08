CREATE TABLE prepaid_orders (
 id text PRIMARY KEY,
 organization_id text NOT NULL,
 server_id text NOT NULL REFERENCES logical_servers(id),
 revision_id text NOT NULL REFERENCES server_revisions(id),
 placement_epoch bigint NOT NULL CHECK(placement_epoch > 0),
 quote text NOT NULL,
 status text NOT NULL CHECK(status IN ('pending','paid','cancelled')),
 created_at_ms bigint NOT NULL,
 expires_at_ms bigint NOT NULL CHECK(expires_at_ms > created_at_ms),
 actor_id text NOT NULL,
 idempotency_key text NOT NULL,
 request_hash text NOT NULL,
 UNIQUE(organization_id,idempotency_key)
);
CREATE INDEX prepaid_orders_tenant_created ON prepaid_orders(organization_id,created_at_ms,id);
