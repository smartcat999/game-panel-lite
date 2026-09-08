package store

import (
	"context"
	"errors"
	"math"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RegisterRegion is for a trusted operator composition, not tenant requests.
// Registration never provisions infrastructure and starts closed to new creates.
func (s *Store) RegisterRegion(ctx context.Context, id, name string) (regions.Entry, error) {
	if err := regions.ValidateIdentity(id, name); err != nil {
		return regions.Entry{}, err
	}
	entry := regions.Entry{ID: id, Name: name, Version: 1}
	result := s.db.WithContext(ctx).Table("global_regions").Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
	if result.Error != nil {
		return regions.Entry{}, result.Error
	}
	if result.RowsAffected != 1 {
		return regions.Entry{}, regions.ErrRegionConflict
	}
	return entry, nil
}

func (s *Store) GetRegion(ctx context.Context, id string) (regions.Entry, error) {
	var entry regions.Entry
	err := s.db.WithContext(ctx).Table("global_regions").Where("id = ?", id).Take(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return entry, err
}

// ListRegions uses a stable ID cursor and never reads nodes or regional metrics.
func (s *Store) ListRegions(ctx context.Context, after string, limit int) ([]regions.Entry, error) {
	if limit < 1 || limit > 100 {
		return nil, regions.ErrInvalidRegion
	}
	var entries []regions.Entry
	err := s.db.WithContext(ctx).Table("global_regions").Where("id > ?", after).Order("id").Limit(limit).Find(&entries).Error
	return entries, err
}

// SetRegionAcceptingCreates uses CAS to prevent stale operator decisions from
// reopening a closed region. Closing does not stop existing deployments.
func (s *Store) SetRegionAcceptingCreates(ctx context.Context, id string, version int64, accept bool) error {
	if version < 1 || version == math.MaxInt64 {
		return regions.ErrInvalidRegion
	}
	result := s.db.WithContext(ctx).Table("global_regions").Where("id = ? AND version = ?", id, version).Updates(map[string]any{"accepting_creates": accept, "version": version + 1})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return regions.ErrRegionConflict
	}
	return nil
}

func migrateSQLiteRegionDirectory(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 5").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(regionDirectorySQL).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(5)").Error
	})
}
