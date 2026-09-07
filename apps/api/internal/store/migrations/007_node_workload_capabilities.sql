-- Unknown/older agents remain ineligible until they advertise configured support.
ALTER TABLE compute_nodes ADD COLUMN workload_capabilities text;
