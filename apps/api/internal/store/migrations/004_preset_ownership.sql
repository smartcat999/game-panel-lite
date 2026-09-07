-- Legacy presets stay unowned; never infer a tenant from the first account.
ALTER TABLE config_presets ADD COLUMN organization_id text NOT NULL DEFAULT '';
CREATE INDEX idx_config_presets_organization_id ON config_presets (organization_id);
ALTER TABLE config_presets ADD COLUMN revision bigint NOT NULL DEFAULT 0;
