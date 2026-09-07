package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

func (s *Store) userServers(ctx context.Context, userID string) *gorm.DB {
	membership := s.db.Model(&domain.OrganizationMember{}).Select("1").
		Where("organization_members.organization_id = game_servers.organization_id AND organization_members.user_id = ? AND organization_members.user_id <> ''", userID).
		Where("organization_members.role IN ?", []domain.Role{domain.RoleOwner, domain.RoleAdmin, domain.RoleMember, domain.RoleViewer})
	return s.db.WithContext(ctx).Model(&domain.GameServer{}).Where("EXISTS (?)", membership)
}

func (s *Store) ListUserGameServers(ctx context.Context, userID string) ([]domain.GameServer, error) {
	servers := []domain.GameServer{}
	err := s.userServers(ctx, userID).Order("created_at DESC, id ASC").Find(&servers).Error
	return servers, err
}

func (s *Store) ListUserGameServersPage(ctx context.Context, userID string, options GameServerListOptions) (GameServerPage, error) {
	return s.listGameServersPage(s.userServers(ctx, userID), options)
}

func (s *Store) ServerMembershipRole(ctx context.Context, userID, serverID string) (domain.Role, error) {
	var member domain.OrganizationMember
	err := s.db.WithContext(ctx).Model(&domain.OrganizationMember{}).
		Joins("JOIN game_servers ON game_servers.organization_id = organization_members.organization_id").
		Select("organization_members.*").Where("game_servers.id = ? AND organization_members.user_id = ? AND organization_members.user_id <> ''", serverID, userID).Take(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	return member.Role, err
}

func (s *Store) GetUserGameServer(ctx context.Context, userID, id string) (domain.GameServer, error) {
	var server domain.GameServer
	err := s.userServers(ctx, userID).Where("id = ?", id).Take(&server).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return server, err
}
