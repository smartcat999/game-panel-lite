CREATE TABLE global_assets (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES organizations(id)
);
CREATE INDEX idx_global_assets_owner ON global_assets(organization_id,id);
CREATE TABLE global_asset_versions (
 asset_id text NOT NULL REFERENCES global_assets(id),
 version text NOT NULL,
 sha256 text NOT NULL CHECK (length(sha256) = 64),
 size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
 PRIMARY KEY(asset_id,version)
);
