package store

import (
	"encoding/json"
	"fmt"

	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Preserve all historical owners, including conflicting and superseded tasks.
// Caller owns the migration transaction; malformed bindings must roll it back.
func backfillNodePorts(tx *gorm.DB) error {
	for _, table := range []string{"game_servers", "workload_assignments"} {
		lastID, first := "", true
		for {
			var rows []struct {
				ID, NodeID, Spec string
				ServerID         *string
			}
			columns := "id,id AS server_id,node_id,spec"
			if table == "workload_assignments" {
				columns = "id,server_id,node_id,spec"
			}
			query := tx.Table(table).Select(columns).Order("id").Limit(idLookupBatchSize)
			if !first {
				query = query.Where("id > ?", lastID)
			}
			if err := query.Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) == 0 {
				break
			}
			claims := []nodePortReservation{}
			pools := []nodePortPool{}
			seenNodes := map[string]bool{}
			for _, row := range rows {
				var spec struct {
					Network workload.Network `json:"network"`
				}
				if row.Spec != "" {
					if err := json.Unmarshal([]byte(row.Spec), &spec); err != nil {
						return fmt.Errorf("invalid network manifest in %s record %s", table, row.ID)
					}
				}
				if row.NodeID == "" {
					continue
				}
				bindings := []workload.Port{{Port: spec.Network.Port, HostPort: spec.Network.HostPort}}
				if table == "workload_assignments" {
					bindings = append(bindings, spec.Network.AdditionalPorts...)
				}
				for _, binding := range bindings {
					port := binding.HostPort
					if port == 0 {
						port = binding.Port
					}
					if port == 0 {
						continue
					}
					if port < 1 || port > 65535 {
						return fmt.Errorf("invalid host port in %s record %s", table, row.ID)
					}
					if row.ServerID == nil {
						return fmt.Errorf("missing port owner in %s record %s", table, row.ID)
					}
					claims = append(claims, nodePortReservation{NodeID: row.NodeID, ServerID: *row.ServerID, HostPort: port})
					if !seenNodes[row.NodeID] {
						seenNodes[row.NodeID] = true
						pools = append(pools, nodePortPool{NodeID: row.NodeID})
					}
				}
			}
			if len(claims) != 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(claims, 256).Error; err != nil {
					return err
				}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(pools, 256).Error; err != nil {
					return err
				}
			}
			lastID, first = rows[len(rows)-1].ID, false
		}
	}
	return nil
}
