ALTER TABLE regional_allocations ADD COLUMN ports jsonb NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(ports) = 'array' AND jsonb_array_length(ports) <= 128);
CREATE TABLE regional_port_reservations (
 allocation_id text NOT NULL REFERENCES regional_allocations(id),
 node_id text NOT NULL REFERENCES regional_nodes(id),
 host_port integer NOT NULL CHECK(host_port BETWEEN 1 AND 65535),
 container_port integer NOT NULL CHECK(container_port BETWEEN 1 AND 65535),
 protocol text NOT NULL CHECK(protocol IN ('tcp','udp')),
 status text NOT NULL CHECK(status IN ('reserved','released')),
 PRIMARY KEY(allocation_id,host_port,protocol)
);
CREATE UNIQUE INDEX idx_regional_port_active ON regional_port_reservations(node_id,host_port,protocol) WHERE status = 'reserved';
