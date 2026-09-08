CREATE TABLE service_subscriptions (
 id text PRIMARY KEY,
 organization_id text NOT NULL,
 server_id text NOT NULL UNIQUE REFERENCES logical_servers(id),
 order_id text NOT NULL UNIQUE REFERENCES prepaid_orders(id),
 payment_id text NOT NULL UNIQUE REFERENCES prepaid_payment_captures(id),
 revision_id text NOT NULL REFERENCES server_revisions(id),
 placement_epoch bigint NOT NULL CHECK(placement_epoch > 0),
 quote text NOT NULL,
 status text NOT NULL CHECK(status IN ('pending_activation','active','cancelled')),
 created_at_ms bigint NOT NULL
);
ALTER TABLE prepaid_fulfillment_tasks ADD COLUMN subscription_id text REFERENCES service_subscriptions(id);
