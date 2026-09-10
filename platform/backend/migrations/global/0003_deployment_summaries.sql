CREATE TABLE deployment_summaries (
    logical_instance_id text PRIMARY KEY,
    regional_deployment_id text NOT NULL,
    region_id text NOT NULL,
    sequence bigint NOT NULL,
    observed_state text NOT NULL,
    observed_at timestamptz NOT NULL
);
CREATE INDEX deployment_summaries_region_id_idx ON deployment_summaries (region_id, logical_instance_id);

CREATE TABLE region_operator_scopes (
    user_id text NOT NULL,
    region_id text NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, region_id)
);
CREATE INDEX region_operator_scopes_region_id_idx ON region_operator_scopes (region_id, user_id);
