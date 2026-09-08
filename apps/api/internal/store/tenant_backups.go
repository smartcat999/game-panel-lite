package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

func (s *Store) userBackups(ctx context.Context, userID string) *gorm.DB {
	query := s.db.WithContext(ctx).Model(&domain.Backup{})
	var ids []string
	if err := s.userServers(ctx, userID).Pluck("id", &ids).Error; err != nil {
		query.AddError(err)
		return query
	}
	return s.whereIDs(query, "instance_id", ids)
}

func (s *Store) ListUserBackups(ctx context.Context, userID string) ([]domain.Backup, error) {
	backups := []domain.Backup{}
	err := s.readSnapshot(ctx, func(tx *Store) error {
		return tx.userBackups(ctx, userID).Order("created_at DESC, id ASC").Find(&backups).Error
	})
	return backups, err
}

func (s *Store) GetUserBackup(ctx context.Context, userID, backupID string) (domain.Backup, error) {
	var backup domain.Backup
	err := s.readSnapshot(ctx, func(tx *Store) error {
		return tx.userBackups(ctx, userID).Where("id = ?", backupID).Take(&backup).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return backup, ErrNotFound
	}
	return backup, err
}
