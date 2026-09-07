ALTER TABLE worlds ADD COLUMN organization_id text NOT NULL DEFAULT '';
UPDATE worlds SET organization_id = game_servers.organization_id
FROM game_servers WHERE worlds.instance_id = game_servers.id AND game_servers.organization_id IS NOT NULL;
CREATE INDEX idx_worlds_organization_id ON worlds (organization_id);
