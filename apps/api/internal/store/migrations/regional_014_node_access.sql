CREATE TABLE regional_node_access (
 organization_id text PRIMARY KEY,
 node_ids jsonb NOT NULL CHECK(jsonb_typeof(node_ids) = 'array' AND jsonb_array_length(node_ids) <= 200),
 enabled boolean NOT NULL DEFAULT false,
 version bigint NOT NULL CHECK(version > 0)
);
