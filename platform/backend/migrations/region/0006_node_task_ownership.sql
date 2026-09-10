ALTER TABLE regional_instance_tasks ADD COLUMN node_id text NOT NULL DEFAULT '';
CREATE INDEX regional_instance_tasks_node_claim_idx ON regional_instance_tasks (node_id, status, claim_until, created_at, id);
