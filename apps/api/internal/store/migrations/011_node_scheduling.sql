-- Existing nodes remain eligible until an administrator explicitly drains them.
ALTER TABLE compute_nodes ADD COLUMN unschedulable boolean NOT NULL DEFAULT false;
