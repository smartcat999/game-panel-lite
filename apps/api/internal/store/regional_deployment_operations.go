package store

import (
	"context"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
)

// ListRegionalDeploymentOperations returns the Region-owned execution view.
// Allocations are fetched by deployment ID in a separate batched query.
func (s *RegionalStore) ListRegionalDeploymentOperations(ctx context.Context, after string, limit int) (regional.DeploymentOperationsPage, error) {
	if limit < 1 || limit > 200 || len(after) > 128 || after != strings.TrimSpace(after) {
		return regional.DeploymentOperationsPage{}, regional.ErrInvalidDeploymentOperations
	}
	page := regional.DeploymentOperationsPage{RegionID: s.regionID, Deployments: make([]regional.DeploymentOperations, 0)}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		page.ObservedAtMS = now
		var rows []regionalSchedulingRow
		if err := tx.Table("regional_deployments").Where("id > ?", after).Order("id").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			page.NextCursor = rows[len(rows)-1].ID
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]string, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		var allocations []struct{ DeploymentID, NodeID string }
		if err := tx.Table("regional_allocations").Select("deployment_id,node_id").Where("deployment_id IN ? AND status = ?", ids, "reserved").Find(&allocations).Error; err != nil {
			return err
		}
		nodeByDeployment := make(map[string]string, len(allocations))
		for _, allocation := range allocations {
			if nodeByDeployment[allocation.DeploymentID] != "" {
				return regional.ErrInvalidDeploymentOperations
			}
			nodeByDeployment[allocation.DeploymentID] = allocation.NodeID
		}
		for _, row := range rows {
			page.Deployments = append(page.Deployments, regional.DeploymentOperations{ID: row.ID, OrganizationID: row.OrganizationID, ServerID: row.ServerID, PlacementEpoch: row.PlacementEpoch, RevisionID: row.RevisionID, SpecGeneration: row.SpecGeneration, IntentVersion: row.IntentVersion, DesiredState: row.DesiredState, SchedulingStatus: row.SchedulingStatus, NodeID: nodeByDeployment[row.ID]})
		}
		return nil
	})
	if err != nil {
		return regional.DeploymentOperationsPage{}, err
	}
	if err := page.Validate(); err != nil {
		return regional.DeploymentOperationsPage{}, err
	}
	return page, nil
}
