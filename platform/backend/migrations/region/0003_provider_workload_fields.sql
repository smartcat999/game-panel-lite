ALTER TABLE regional_deployments
    ADD COLUMN game_version text NOT NULL DEFAULT '',
    ADD COLUMN configuration jsonb NOT NULL DEFAULT '{}'::jsonb;
