ALTER TABLE organizations ADD COLUMN credits bigint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS credit_transactions (
    id text PRIMARY KEY,
    organization_id text NOT NULL,
    amount bigint NOT NULL,
    balance_after bigint NOT NULL,
    type text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_credit_transactions_org ON credit_transactions USING btree (organization_id);

CREATE TABLE IF NOT EXISTS oauth_identities (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    provider text NOT NULL,
    provider_user_id text NOT NULL,
    email text NOT NULL DEFAULT '',
    name text NOT NULL DEFAULT '',
    avatar_url text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_provider_user ON oauth_identities USING btree (provider, provider_user_id);
CREATE INDEX idx_oauth_identities_user ON oauth_identities USING btree (user_id);
