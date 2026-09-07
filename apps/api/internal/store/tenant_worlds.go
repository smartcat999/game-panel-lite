package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

func (s *Store) userWorlds(ctx context.Context, userID string) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.World{}).Where("organization_id IN (?)", s.userOrganizations(ctx, userID).Select("organizations.id"))
}
func (s *Store) ListUserWorlds(ctx context.Context, userID string) ([]domain.World, error) {
	worlds := []domain.World{}
	err := s.userWorlds(ctx, userID).Order("created_at DESC, id ASC").Find(&worlds).Error
	for i := range worlds {
		hydrateWorldConfigPayload(&worlds[i])
	}
	return worlds, err
}
func (s *Store) GetUserWorld(ctx context.Context, userID, id string) (domain.World, error) {
	var world domain.World
	err := s.userWorlds(ctx, userID).Where("id = ?", id).Take(&world).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return world, ErrNotFound
	}
	hydrateWorldConfigPayload(&world)
	return world, err
}
func (s *Store) GetWorldByOrganizationInstanceAndFile(ctx context.Context, orgID, instanceID, fileName string) (domain.World, error) {
	var world domain.World
	err := s.db.WithContext(ctx).Where("organization_id = ? AND instance_id = ? AND file_name = ?", orgID, instanceID, fileName).Take(&world).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return world, ErrNotFound
	}
	hydrateWorldConfigPayload(&world)
	return world, err
}
