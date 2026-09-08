package store

import (
	"context"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

type artifactReference struct {
	AssignmentID   string `gorm:"primaryKey"`
	ArtifactID     string `gorm:"primaryKey"`
	OrganizationID string
}

func (artifactReference) TableName() string { return "workload_artifact_references" }

// Called in the same transaction as the assignment write, after its ID resolves.
func (s *Store) replaceArtifactReferences(ctx context.Context, orgID string, assignment domain.WorkloadAssignment) error {
	if err := s.db.WithContext(ctx).Where("assignment_id = ?", assignment.ID).Delete(&artifactReference{}).Error; err != nil {
		return err
	}
	seen := map[string]bool{}
	rows := make([]artifactReference, 0, len(assignment.Spec.Options.Artifacts))
	for _, ref := range assignment.Spec.Options.Artifacts {
		if seen[ref.ID] {
			continue
		}
		seen[ref.ID] = true
		rows = append(rows, artifactReference{AssignmentID: assignment.ID, ArtifactID: ref.ID, OrganizationID: orgID})
	}
	if len(rows) == 0 {
		return nil
	}
	// Three bound columns per row; stay below SQLite's portable variable limit.
	return s.db.WithContext(ctx).CreateInBatches(rows, 256).Error
}

// SQLite still uses the legacy schema initializer. This ledger makes reference
// backfill a one-time atomic migration rather than a scan on every startup.
func migrateSQLiteArtifactReferences(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE TABLE IF NOT EXISTS gamepanel_sqlite_migrations (version integer PRIMARY KEY)").Error; err != nil {
			return err
		}
		var applied int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 1").Count(&applied).Error; err != nil {
			return err
		}
		if applied != 0 {
			return nil
		}
		for _, statement := range []string{
			`CREATE TABLE workload_artifact_references (assignment_id text NOT NULL REFERENCES workload_assignments(id) ON DELETE CASCADE, artifact_id text NOT NULL, organization_id text NOT NULL, PRIMARY KEY(assignment_id,artifact_id))`,
			`CREATE INDEX idx_workload_artifact_references_owner_source ON workload_artifact_references(organization_id,artifact_id)`,
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		if err := backfillArtifactReferences(tx); err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES (1)").Error
	})
}
