package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

func (s *Store) userBackups(ctx context.Context, userID string) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.Backup{}).
		Where("instance_id IN (?)", s.userServers(ctx, userID).Select("game_servers.id"))
}

func (s *Store) ListUserBackups(ctx context.Context, userID string) ([]domain.Backup, error) {
	backups := []domain.Backup{}
	err := s.userBackups(ctx, userID).Order("created_at DESC, id ASC").Find(&backups).Error
	return backups, err
}

func (s *Store) GetUserBackup(ctx context.Context, userID, backupID string) (domain.Backup, error) {
	var backup domain.Backup
	err := s.userBackups(ctx, userID).Where("id = ?", backupID).Take(&backup).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return backup, ErrNotFound
	}
	return backup, err
}
