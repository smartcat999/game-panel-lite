ALTER TABLE service_subscriptions DROP CONSTRAINT IF EXISTS service_subscriptions_status_check;
ALTER TABLE service_subscriptions ADD CONSTRAINT service_subscriptions_status_check CHECK(status IN ('pending_activation','active','expired','cancelled'));
