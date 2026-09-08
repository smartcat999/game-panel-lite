ALTER TABLE prepaid_orders ADD COLUMN cancel_reason text NOT NULL DEFAULT '' CHECK(cancel_reason IN ('','user','expired'));
ALTER TABLE prepaid_orders ADD COLUMN cancelled_at_ms bigint NOT NULL DEFAULT 0 CHECK(cancelled_at_ms >= 0);
ALTER TABLE prepaid_orders ADD COLUMN cancelled_by text NOT NULL DEFAULT '';
CREATE INDEX prepaid_orders_pending_expiry ON prepaid_orders(expires_at_ms,id) WHERE status = 'pending';
