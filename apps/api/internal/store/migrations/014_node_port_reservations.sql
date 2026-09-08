CREATE TABLE node_port_pools (node_id text PRIMARY KEY);
CREATE TABLE node_port_reservations (
 node_id text NOT NULL,
 host_port integer NOT NULL CHECK (host_port BETWEEN 1 AND 65535),
 server_id text NOT NULL,
 PRIMARY KEY (node_id, host_port, server_id)
);
CREATE INDEX idx_node_port_reservations_server ON node_port_reservations(server_id);
-- Preserve every historical owner of conflicting ports; never pick a winner.
WITH bindings AS (
 SELECT node_id, id AS server_id, COALESCE(NULLIF((NULLIF(spec,'')::jsonb #>> '{network,hostPort}')::integer,0),(NULLIF(spec,'')::jsonb #>> '{network,port}')::integer,0) AS host_port FROM game_servers
 UNION ALL
 SELECT node_id, server_id, COALESCE(NULLIF((NULLIF(spec,'')::jsonb #>> '{network,hostPort}')::integer,0),(NULLIF(spec,'')::jsonb #>> '{network,port}')::integer,0) FROM workload_assignments
 UNION ALL
 SELECT a.node_id, a.server_id, COALESCE(NULLIF((p->>'hostPort')::integer,0),(p->>'port')::integer,0) FROM workload_assignments a CROSS JOIN LATERAL jsonb_array_elements(COALESCE(NULLIF(a.spec,'')::jsonb #> '{network,additionalPorts}','[]'::jsonb)) p
)
INSERT INTO node_port_reservations(node_id,host_port,server_id)
SELECT DISTINCT node_id,host_port,server_id FROM bindings WHERE COALESCE(node_id,'')<>'' AND host_port<>0
ON CONFLICT DO NOTHING;
INSERT INTO node_port_pools(node_id) SELECT DISTINCT node_id FROM node_port_reservations ON CONFLICT DO NOTHING;
