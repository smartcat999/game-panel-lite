package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

// ListUserActivity filters before limiting so foreign traffic cannot displace
// the caller's history. An empty instance ID lists all of their workspaces.
func (s *Store) ListUserActivity(ctx context.Context, userID, instanceID string, limit int) ([]domain.ActivityEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	events := []domain.ActivityEvent{}
	err := s.readSnapshot(ctx, func(tx *Store) error {
		query := tx.userOwnedQuery(ctx, userID, &domain.ActivityEvent{})
		if instanceID != "" {
			query = query.Where("instance_id = ?", instanceID)
		}
		return query.Order(tx.creationOrder(true)).Limit(limit).Find(&events).Error
	})
	for i := range events {
		hydrateActivityPayload(&events[i])
	}
	return events, err
}

// Instance monitoring shows only history owned by the current instance space.
// Platform-wide audit reads remain available through the separate admin path.
func (s *Store) ListCurrentInstanceActivity(ctx context.Context, instanceID string, limit int) ([]domain.ActivityEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	events := []domain.ActivityEvent{}
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var server struct{ OrganizationID *string }
		err := tx.db.WithContext(ctx).Table("game_servers").Select("organization_id").Where("id = ?", instanceID).Take(&server).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return tx.db.WithContext(ctx).Model(&domain.ActivityEvent{}).
			Where("instance_id = ? AND organization_id = ?", instanceID, server.OrganizationID).
			Order(tx.creationOrder(true)).Limit(limit).Find(&events).Error
	})
	for i := range events {
		hydrateActivityPayload(&events[i])
	}
	return events, err
}
