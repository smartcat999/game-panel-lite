package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&domain.GameServerInstance{}, &domain.Backup{}, &domain.World{}, &domain.ModFile{}, &domain.ActivityEvent{}); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) CreateServer(ctx context.Context, server *domain.GameServerInstance) error {
	return s.db.WithContext(ctx).Create(server).Error
}

func (s *Store) SaveServer(ctx context.Context, server *domain.GameServerInstance) error {
	return s.db.WithContext(ctx).Save(server).Error
}

func (s *Store) ListServers(ctx context.Context) ([]domain.GameServerInstance, error) {
	var servers []domain.GameServerInstance
	return servers, s.db.WithContext(ctx).Order("created_at desc").Find(&servers).Error
}

func (s *Store) GetServer(ctx context.Context, id string) (domain.GameServerInstance, error) {
	var server domain.GameServerInstance
	err := s.db.WithContext(ctx).First(&server, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return server, ErrNotFound
	}
	return server, err
}

func (s *Store) DeleteServer(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.GameServerInstance{}, "id = ?", id).Error
}

var ErrNotFound = errors.New("not found")
