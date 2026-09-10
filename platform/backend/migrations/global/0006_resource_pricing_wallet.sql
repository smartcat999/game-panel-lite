CREATE TABLE region_resource_catalogs (
    id text PRIMARY KEY,
    region_id text NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    cpu_min_milli bigint NOT NULL,
    cpu_max_milli bigint NOT NULL,
    cpu_step_milli bigint NOT NULL,
    memory_min_mib bigint NOT NULL,
    memory_max_mib bigint NOT NULL,
    memory_step_mib bigint NOT NULL,
    disk_min_gib bigint NOT NULL,
    disk_max_gib bigint NOT NULL,
    disk_step_gib bigint NOT NULL,
    dedicated_ip_available boolean NOT NULL,
    endpoint_delivery_modes jsonb NOT NULL,
    availability text NOT NULL,
    effective_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (region_id, version)
);
CREATE INDEX region_resource_catalogs_effective_idx ON region_resource_catalogs (region_id, effective_at DESC);

CREATE TABLE price_books (
    id text PRIMARY KEY,
    region_id text NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    currency text NOT NULL CHECK (currency = 'CNY'),
    unit_prices jsonb NOT NULL,
    effective_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (region_id, revision)
);
CREATE INDEX price_books_effective_idx ON price_books (region_id, effective_at DESC);

CREATE TABLE resource_quotes (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    region_id text NOT NULL,
    price_book_id text NOT NULL,
    cpu_milli bigint NOT NULL,
    memory_mib bigint NOT NULL,
    disk_gib bigint NOT NULL,
    dedicated_ip boolean NOT NULL,
    estimated_hourly_minor bigint NOT NULL CHECK (estimated_hourly_minor >= 0),
    currency text NOT NULL CHECK (currency = 'CNY'),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    hold_id text UNIQUE,
    hold_status text,
    hold_expires_at timestamptz,
    funding_authorization_id text UNIQUE,
    funding_maximum_debit_minor bigint,
    funding_currency text,
    funding_expires_at timestamptz,
    funding_signature text
);
CREATE INDEX resource_quotes_workspace_idx ON resource_quotes (workspace_id, created_at DESC);
CREATE INDEX resource_quotes_expiry_idx ON resource_quotes (expires_at);

CREATE TABLE wallets (
    workspace_id text PRIMARY KEY,
    currency text NOT NULL CHECK (currency = 'CNY'),
    ledger_sequence bigint NOT NULL DEFAULT 0,
    promotional_minor bigint NOT NULL DEFAULT 0 CHECK (promotional_minor >= 0),
    cash_minor bigint NOT NULL DEFAULT 0 CHECK (cash_minor >= 0),
    state text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE ledger_entries (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    sequence bigint NOT NULL,
    kind text NOT NULL,
    bucket text NOT NULL,
    amount_minor bigint NOT NULL CHECK (amount_minor <> 0),
    source_id text NOT NULL,
    reason text NOT NULL,
    occurred_at timestamptz NOT NULL,
    UNIQUE (workspace_id, sequence),
    UNIQUE (workspace_id, kind, bucket, source_id)
);
CREATE INDEX ledger_entries_workspace_idx ON ledger_entries (workspace_id, sequence);

CREATE TABLE usage_records (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    logical_instance_id text NOT NULL,
    region_id text NOT NULL,
    price_book_id text NOT NULL,
    resource_kind text NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    interval_start timestamptz NOT NULL,
    interval_end timestamptz NOT NULL,
    charge_minor bigint NOT NULL CHECK (charge_minor >= 0),
    created_at timestamptz NOT NULL
);
CREATE INDEX usage_records_workspace_interval_idx ON usage_records (workspace_id, interval_start, interval_end);
CREATE INDEX usage_records_instance_interval_idx ON usage_records (logical_instance_id, interval_start, interval_end);

CREATE FUNCTION reject_immutable_financial_row() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'immutable financial row';
END;
$$;

CREATE TRIGGER price_books_immutable BEFORE UPDATE OR DELETE ON price_books
FOR EACH ROW EXECUTE FUNCTION reject_immutable_financial_row();
CREATE TRIGGER region_resource_catalogs_immutable BEFORE UPDATE OR DELETE ON region_resource_catalogs
FOR EACH ROW EXECUTE FUNCTION reject_immutable_financial_row();
CREATE TRIGGER ledger_entries_immutable BEFORE UPDATE OR DELETE ON ledger_entries
FOR EACH ROW EXECUTE FUNCTION reject_immutable_financial_row();
CREATE TRIGGER usage_records_immutable BEFORE UPDATE OR DELETE ON usage_records
FOR EACH ROW EXECUTE FUNCTION reject_immutable_financial_row();
