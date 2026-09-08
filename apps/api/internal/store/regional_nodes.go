package store

import (
	"context"
	"math"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConfigureRegionalNode replaces only operator-owned fields. Its database is
// bound to one Region at open time. Node liveness and allocation are separate.
// expectedVersion=0 creates; subsequent writes require the observed version.
func (s *RegionalStore) ConfigureRegionalNode(ctx context.Context, config regional.NodeConfiguration, expectedVersion int64) (regional.Node, error) {
	if config.Validate() != nil || expectedVersion < 0 || expectedVersion == math.MaxInt64 {
		return regional.Node{}, regional.ErrInvalidNode
	}
	var node regional.Node
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if expectedVersion == 0 {
			node = regional.Node{NodeConfiguration: config, Version: 1}
			result := tx.Table("regional_nodes").Clauses(clause.OnConflict{DoNothing: true}).Create(&node)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return regional.ErrNodeVersionConflict
			}
			return nil
		}
		result := tx.Table("regional_nodes").Where("id = ? AND version = ?", config.ID, expectedVersion).Updates(map[string]any{"name": config.Name, "architecture": config.Architecture, "cpu": config.CPU, "memory_mb": config.MemoryMB, "schedulable": config.Schedulable, "version": expectedVersion + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return regional.ErrNodeVersionConflict
		}
		return tx.Table("regional_nodes").Where("id = ?", config.ID).Take(&node).Error
	})
	if err != nil {
		return regional.Node{}, err
	}
	return node, nil
}

// ListRegionalNodes provides bounded keyset pagination for trusted regional
// management. It is not a tenant-visible host list or a scheduling candidate set.
func (s *RegionalStore) ListRegionalNodes(ctx context.Context, afterID string, limit int) ([]regional.Node, error) {
	if limit < 1 || limit > 200 || len(afterID) > 128 {
		return nil, regional.ErrInvalidNode
	}
	nodes := make([]regional.Node, 0)
	err := s.db.WithContext(ctx).Table("regional_nodes").Where("id > ?", afterID).Order("id").Limit(limit).Find(&nodes).Error
	return nodes, err
}
