package store

import (
	"context"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

// NodeConfigurationPatch contains only administrator-owned configuration.
// Nil leaves a field unchanged; a pointer to an empty string clears it.
type NodeConfigurationPatch struct {
	Name     *string
	Region   *string
	PublicIP *string
	Host     *string
	Port     *int
}

func (s *Store) UpdateNodeConfiguration(ctx context.Context, id string, patch NodeConfigurationPatch) (domain.ComputeNode, error) {
	updates := map[string]any{}
	for column, value := range map[string]*string{"name": patch.Name, "region": patch.Region, "public_ip": patch.PublicIP, "host": patch.Host} {
		if value != nil {
			updates[column] = *value
		}
	}
	if patch.Port != nil {
		updates["port"] = *patch.Port
	}
	if len(updates) == 0 {
		return s.GetComputeNode(ctx, id)
	}
	updates["updated_at"] = time.Now().UTC()
	var node domain.ComputeNode
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&domain.ComputeNode{}).Where("id = ?", id).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		var err error
		node, err = (&Store{db: tx}).GetComputeNode(ctx, id)
		return err
	})
	return node, err
}
