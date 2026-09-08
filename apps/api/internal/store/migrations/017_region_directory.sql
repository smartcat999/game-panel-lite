CREATE TABLE global_regions (
 id text PRIMARY KEY,
 name text NOT NULL,
 accepting_creates boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);
