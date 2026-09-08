package store

import "gorm.io/gorm"

// Historical SQL remains the checksum identity for already deployed databases.
// Only exact known scripts receive equivalent ID-based execution; modified or
// synthetic migrations must still execute their supplied SQL and fail normally.
func executeMigration(tx *gorm.DB, migration sqlMigration) error {
	if migration.version == 6 && migration.name == "artifact_references" && migration.sql == artifactReferencesSQL {
		if err := tx.Exec(`CREATE TABLE workload_artifact_references (
assignment_id text NOT NULL REFERENCES workload_assignments(id) ON DELETE CASCADE,
artifact_id text NOT NULL, organization_id text NOT NULL,
PRIMARY KEY (assignment_id, artifact_id));
CREATE INDEX idx_workload_artifact_references_owner_source ON workload_artifact_references (organization_id, artifact_id);`).Error; err != nil {
			return err
		}
		return backfillArtifactReferences(tx)
	}
	if migration.version == 14 && migration.name == "node_port_reservations" && migration.sql == nodePortReservationsSQL {
		if err := tx.Exec(`CREATE TABLE node_port_pools (node_id text PRIMARY KEY);
CREATE TABLE node_port_reservations (
node_id text NOT NULL, host_port integer NOT NULL CHECK (host_port BETWEEN 1 AND 65535),
server_id text NOT NULL, PRIMARY KEY (node_id,host_port,server_id));
CREATE INDEX idx_node_port_reservations_server ON node_port_reservations(server_id);`).Error; err != nil {
			return err
		}
		return backfillNodePorts(tx)
	}
	return tx.Exec(migration.sql).Error
}
