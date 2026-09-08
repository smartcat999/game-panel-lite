package store

import (
	"strings"

	"gorm.io/gorm"
)

// The table is an internal migration target, never request input. Read source
// ownership in batches, then update only the rows still lacking an owner.
func backfillInstanceOwnership(tx *gorm.DB, table string) error {
	lastID, first := "", true
	for {
		var rows []struct {
			ID         string
			InstanceID *string
		}
		query := tx.Table(table).Select("id,instance_id").Where("organization_id = '' OR organization_id IS NULL").Order("id").Limit(idLookupBatchSize)
		if !first {
			query = query.Where("id > ?", lastID)
		}
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := []string{}
		seen := map[string]bool{}
		for _, row := range rows {
			if row.InstanceID != nil && !seen[*row.InstanceID] {
				seen[*row.InstanceID] = true
				ids = append(ids, *row.InstanceID)
			}
		}
		var instances []struct{ ID, OrganizationID string }
		if len(ids) != 0 {
			if err := tx.Table("game_servers").Select("id,organization_id").Where("id IN ? AND organization_id IS NOT NULL", ids).Find(&instances).Error; err != nil {
				return err
			}
		}
		owners := map[string]string{}
		for _, instance := range instances {
			owners[instance.ID] = instance.OrganizationID
		}
		// Each row needs three parameters (CASE key, value, WHERE key).
		for start := 0; start < len(rows); start += 256 {
			var expression strings.Builder
			expression.WriteString("CASE id")
			args := []any{}
			targetIDs := []string{}
			for _, row := range rows[start:min(start+256, len(rows))] {
				if row.InstanceID == nil {
					continue
				}
				owner, found := owners[*row.InstanceID]
				if !found {
					continue
				}
				expression.WriteString(" WHEN ? THEN ?")
				args = append(args, row.ID, owner)
				targetIDs = append(targetIDs, row.ID)
			}
			if len(targetIDs) == 0 {
				continue
			}
			expression.WriteString(" ELSE organization_id END")
			if err := tx.Table(table).Where("id IN ?", targetIDs).Where("organization_id = '' OR organization_id IS NULL").UpdateColumn("organization_id", gorm.Expr(expression.String(), args...)).Error; err != nil {
				return err
			}
		}
		lastID, first = rows[len(rows)-1].ID, false
	}
}
