package store

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type regionalNodeAccessRow struct {
	OrganizationID, NodeIDs string
	Enabled                 bool
	Version                 int64
}

// ConfigureRegionalNodeAccess is a private operator API. IDs may be configured
// before enrollment; unknown nodes never become candidates. Version is CAS only.
func (s *RegionalStore) ConfigureRegionalNodeAccess(ctx context.Context, policy regional.NodeAccessPolicy, expectedVersion int64) (regional.NodeAccessPolicy, error) {
	if policy.Validate() != nil || expectedVersion < 0 || expectedVersion == math.MaxInt64 {
		return regional.NodeAccessPolicy{}, regional.ErrNodeAccessDenied
	}
	nodes := append([]string{}, policy.NodeIDs...)
	encoded, err := json.Marshal(nodes)
	if err != nil {
		return regional.NodeAccessPolicy{}, err
	}
	row := regionalNodeAccessRow{OrganizationID: policy.OrganizationID, NodeIDs: string(encoded), Enabled: policy.Enabled, Version: expectedVersion + 1}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if expectedVersion == 0 {
			result := tx.Table("regional_node_access").Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return regional.ErrNodeAccessVersionConflict
			}
			return nil
		}
		result := tx.Table("regional_node_access").Where("organization_id = ? AND version = ?", policy.OrganizationID, expectedVersion).Updates(map[string]any{"node_ids": row.NodeIDs, "enabled": row.Enabled, "version": row.Version})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return regional.ErrNodeAccessVersionConflict
		}
		return nil
	})
	if err != nil {
		return regional.NodeAccessPolicy{}, err
	}
	return regional.NodeAccessPolicy{OrganizationID: row.OrganizationID, NodeIDs: nodes, Enabled: row.Enabled, Version: row.Version}, nil
}

// regionalNodeAccess optionally holds a policy SHARE lock through admission.
// Policy writers never lock Nodes, keeping the admission lock order acyclic.
func regionalNodeAccess(tx *gorm.DB, organization string, lock bool) (map[string]bool, error) {
	var row regionalNodeAccessRow
	query := tx.Table("regional_node_access").Where("organization_id = ?", organization)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "SHARE"})
	}
	err := query.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, regional.ErrNodeAccessDenied
	}
	if err != nil {
		return nil, err
	}
	policy := regional.NodeAccessPolicy{OrganizationID: row.OrganizationID, Enabled: row.Enabled, Version: row.Version}
	if json.Unmarshal([]byte(row.NodeIDs), &policy.NodeIDs) != nil || policy.Validate() != nil || !policy.Enabled {
		return nil, regional.ErrNodeAccessDenied
	}
	allowed := make(map[string]bool, len(policy.NodeIDs))
	for _, id := range policy.NodeIDs {
		allowed[id] = true
	}
	return allowed, nil
}
