ALTER TABLE activity_events ADD COLUMN organization_id text NOT NULL DEFAULT '';
UPDATE activity_events SET organization_id = game_servers.organization_id
FROM game_servers WHERE activity_events.instance_id = game_servers.id AND game_servers.organization_id IS NOT NULL;
CREATE INDEX idx_activity_events_organization_id ON activity_events (organization_id);
