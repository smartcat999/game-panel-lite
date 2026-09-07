-- Unassigned historical library items have no provable tenant owner.
-- Instance resources retain their existing instance authorization; adoption is separate.
ALTER TABLE mod_files ADD COLUMN organization_id text NOT NULL DEFAULT '';
ALTER TABLE mod_files ADD COLUMN revision bigint NOT NULL DEFAULT 0;
CREATE INDEX idx_mod_files_organization_id ON mod_files (organization_id);
ALTER TABLE mod_packs ADD COLUMN organization_id text NOT NULL DEFAULT '';
ALTER TABLE mod_packs ADD COLUMN revision bigint NOT NULL DEFAULT 0;
CREATE INDEX idx_mod_packs_organization_id ON mod_packs (organization_id);
