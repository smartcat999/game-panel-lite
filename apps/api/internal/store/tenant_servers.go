package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

func (s *Store) userServers(ctx context.Context, userID string) *gorm.DB {
	return s.userOwnedQuery(ctx, userID, &domain.GameServer{})
}

func (s *Store) ListUserGameServers(ctx context.Context, userID string) ([]domain.GameServer, error) {
	servers := []domain.GameServer{}
	err := s.readSnapshot(ctx, func(tx *Store) error {
		return tx.userServers(ctx, userID).Order("created_at DESC, id ASC").Find(&servers).Error
	})
	return servers, err
}

func (s *Store) ListOrganizationGameServers(ctx context.Context, organizationID string) ([]domain.GameServer, error) {
	servers := []domain.GameServer{}
	err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("created_at DESC, id ASC").Find(&servers).Error
	return servers, err
}

func (s *Store) ListUserOrganizationGameServers(ctx context.Context, userID, organizationID string) ([]domain.GameServer, error) {
	servers := []domain.GameServer{}
	err := s.readSnapshot(ctx, func(tx *Store) error {
		return tx.userServers(ctx, userID).Where("organization_id = ?", organizationID).Order("created_at DESC, id ASC").Find(&servers).Error
	})
	return servers, err
}

func (s *Store) ListUserGameServersPage(ctx context.Context, userID string, options GameServerListOptions) (GameServerPage, error) {
	var page GameServerPage
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var err error
		page, err = tx.listGameServersPage(tx.userServers(ctx, userID), options)
		return err
	})
	return page, err
}

func (s *Store) ServerMembershipRole(ctx context.Context, userID, serverID string) (domain.Role, error) {
	var member domain.OrganizationMember
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var server struct{ OrganizationID *string }
		if err := tx.db.WithContext(ctx).Table("game_servers").Select("organization_id").Where("id = ?", serverID).Take(&server).Error; err != nil {
			return err
		}
		return tx.db.WithContext(ctx).Where("organization_id = ? AND user_id = ? AND user_id <> ''", server.OrganizationID, userID).Take(&member).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	return member.Role, err
}

func (s *Store) GetUserGameServer(ctx context.Context, userID, id string) (domain.GameServer, error) {
	var server domain.GameServer
	err := s.readSnapshot(ctx, func(tx *Store) error {
		return tx.userServers(ctx, userID).Where("id = ?", id).Take(&server).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return server, err
}
