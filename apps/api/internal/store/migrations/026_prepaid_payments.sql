CREATE TABLE prepaid_payment_captures (
 id text PRIMARY KEY,
 provider text NOT NULL,
 merchant_id text NOT NULL,
 transaction_id text NOT NULL,
 order_id text NOT NULL REFERENCES prepaid_orders(id),
 amount_minor bigint NOT NULL CHECK(amount_minor > 0),
 currency text NOT NULL,
 paid_at_ms bigint NOT NULL CHECK(paid_at_ms > 0),
 disposition text NOT NULL CHECK(disposition IN ('applied','review')),
 reason text NOT NULL,
 recorded_at_ms bigint NOT NULL,
 UNIQUE(provider,merchant_id,transaction_id)
);
CREATE UNIQUE INDEX prepaid_one_applied_capture ON prepaid_payment_captures(order_id) WHERE disposition = 'applied';
CREATE TABLE prepaid_payment_events (
 provider text NOT NULL,
 merchant_id text NOT NULL,
 event_id text NOT NULL,
 request_hash text NOT NULL,
 payment_id text NOT NULL REFERENCES prepaid_payment_captures(id),
 PRIMARY KEY(provider,merchant_id,event_id)
);
CREATE TABLE prepaid_fulfillment_tasks (
 order_id text PRIMARY KEY REFERENCES prepaid_orders(id),
 payment_id text NOT NULL UNIQUE REFERENCES prepaid_payment_captures(id),
 status text NOT NULL CHECK(status IN ('pending','completed'))
);
