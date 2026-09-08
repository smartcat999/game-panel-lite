package store

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// backfillArtifactReferences runs inside the schema migration transaction.
// Read only the legacy fields needed for ownership, in bounded ID batches.
func backfillArtifactReferences(tx *gorm.DB) error {
	lastID, first := "", true
	for {
		var assignments []struct{ ID, Spec string }
		query := tx.Table("workload_assignments").Select("id,spec").Order("id").Limit(idLookupBatchSize)
		if !first {
			query = query.Where("id > ?", lastID)
		}
		if err := query.Find(&assignments).Error; err != nil {
			return err
		}
		if len(assignments) == 0 {
			return nil
		}
		ids := []string{}
		users := map[string][]string{}
		for _, assignment := range assignments {
			var spec struct {
				Options struct {
					Artifacts []struct {
						ID *string `json:"id"`
					} `json:"artifacts"`
				} `json:"options"`
			}
			if assignment.Spec != "" {
				if err := json.Unmarshal([]byte(assignment.Spec), &spec); err != nil {
					return fmt.Errorf("invalid artifact manifest for assignment %s", assignment.ID)
				}
			}
			seen := map[string]bool{}
			for _, artifact := range spec.Options.Artifacts {
				if artifact.ID == nil || seen[*artifact.ID] {
					continue
				}
				id := *artifact.ID
				seen[id] = true
				if len(users[id]) == 0 {
					ids = append(ids, id)
				}
				users[id] = append(users[id], assignment.ID)
			}
		}
		for start := 0; start < len(ids); start += idLookupBatchSize {
			var mods []struct{ ID, OrganizationID string }
			if err := tx.Table("mod_files").Select("id,organization_id").Where("id IN ? AND organization_id <> ''", ids[start:min(start+idLookupBatchSize, len(ids))]).Find(&mods).Error; err != nil {
				return err
			}
			refs := []artifactReference{}
			for _, mod := range mods {
				for _, assignmentID := range users[mod.ID] {
					refs = append(refs, artifactReference{AssignmentID: assignmentID, ArtifactID: mod.ID, OrganizationID: mod.OrganizationID})
				}
			}
			if len(refs) != 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(refs, 256).Error; err != nil {
					return err
				}
			}
		}
		lastID, first = assignments[len(assignments)-1].ID, false
	}
}
