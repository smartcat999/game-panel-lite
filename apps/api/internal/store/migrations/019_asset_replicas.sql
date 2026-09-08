CREATE TABLE global_asset_replicas (
 id text PRIMARY KEY,
 asset_id text NOT NULL,
 asset_version text NOT NULL,
 region_id text NOT NULL REFERENCES global_regions(id),
 storage_id text NOT NULL,
 available boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
 FOREIGN KEY(asset_id,asset_version) REFERENCES global_asset_versions(asset_id,version),
 UNIQUE(asset_id,asset_version,region_id,storage_id)
);
CREATE INDEX idx_global_asset_replica_sources ON global_asset_replicas(asset_id,asset_version,available,id);
