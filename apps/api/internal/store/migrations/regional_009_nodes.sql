CREATE TABLE regional_nodes (
 id text PRIMARY KEY CHECK(length(id) BETWEEN 1 AND 128),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 128),
 architecture text NOT NULL CHECK(length(architecture) BETWEEN 1 AND 128),
 cpu double precision NOT NULL CHECK(cpu > 0 AND cpu < 'Infinity'::double precision),
 memory_mb bigint NOT NULL CHECK(memory_mb > 0),
 schedulable boolean NOT NULL DEFAULT false,
 version bigint NOT NULL CHECK(version > 0)
);
