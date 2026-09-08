package store

import (
	"context"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type globalAssetRow struct{ ID, OrganizationID string }
type globalAssetVersionRow struct {
	AssetID, Version string
	SHA256           string `gorm:"column:sha256"`
	SizeBytes        int64
}

// PublishAssetVersion is a trusted ingestion boundary. The caller must verify
// the bytes and tenant authority before publishing; tenant HTTP must not accept
// a client-declared digest as verification. Repeating identical metadata is safe.
func (s *Store) PublishAssetVersion(ctx context.Context, v assets.PublishedVersion) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.Transaction(ctx, func(tx *Store) error {
		identity := globalAssetRow{v.AssetID, v.OrganizationID}
		if err := tx.db.WithContext(ctx).Table("global_assets").Clauses(clause.OnConflict{DoNothing: true}).Create(&identity).Error; err != nil {
			return err
		}
		var existing globalAssetRow
		if err := tx.db.WithContext(ctx).Table("global_assets").Where("id = ?", v.AssetID).Take(&existing).Error; err != nil {
			return err
		}
		if existing.OrganizationID != v.OrganizationID {
			return assets.ErrVersionConflict
		}
		version := globalAssetVersionRow{v.AssetID, v.Version, v.SHA256, v.SizeBytes}
		if err := tx.db.WithContext(ctx).Table("global_asset_versions").Clauses(clause.OnConflict{DoNothing: true}).Create(&version).Error; err != nil {
			return err
		}
		var stored globalAssetVersionRow
		if err := tx.db.WithContext(ctx).Table("global_asset_versions").Where("asset_id = ? AND version = ?", v.AssetID, v.Version).Take(&stored).Error; err != nil {
			return err
		}
		if stored != version {
			return assets.ErrVersionConflict
		}
		return nil
	})
}

// checkGlobalAssets runs inside the intent transaction. Published identities and
// versions cannot be changed or deleted, so authorization cannot race transfer.
// Each batch uses two bounded single-table queries, independent of version count.
func (s *Store) checkGlobalAssets(ctx context.Context, organization string, refs []instances.AssetVersion) error {
	const batchSize = 100
	for start := 0; start < len(refs); start += batchSize {
		end := min(start+batchSize, len(refs))
		batch := refs[start:end]
		ids := make([]string, 0, len(batch))
		for _, ref := range batch {
			ids = append(ids, ref.AssetID)
		}
		var identities []globalAssetRow
		if err := s.db.WithContext(ctx).Table("global_assets").Where("organization_id = ? AND id IN ?", organization, ids).Find(&identities).Error; err != nil {
			return err
		}
		if len(identities) != len(batch) {
			return assets.ErrUnavailable
		}
		pairs := s.db.Where("asset_id = ? AND version = ?", batch[0].AssetID, batch[0].Version)
		for _, ref := range batch[1:] {
			pairs = pairs.Or("asset_id = ? AND version = ?", ref.AssetID, ref.Version)
		}
		var versions []globalAssetVersionRow
		if err := s.db.WithContext(ctx).Table("global_asset_versions").Where(pairs).Find(&versions).Error; err != nil {
			return err
		}
		if len(versions) != len(batch) {
			return assets.ErrUnavailable
		}
	}
	return nil
}

func migrateSQLiteGlobalAssets(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 6").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(globalAssetsSQL).Error; err != nil {
			return err
		}
		for _, statement := range []string{
			`CREATE TRIGGER global_assets_no_replace BEFORE INSERT ON global_assets WHEN EXISTS (SELECT 1 FROM global_assets WHERE id = NEW.id AND organization_id <> NEW.organization_id) BEGIN SELECT RAISE(ABORT, 'asset owner is immutable'); END`,
			`CREATE TRIGGER global_asset_versions_no_replace BEFORE INSERT ON global_asset_versions WHEN EXISTS (SELECT 1 FROM global_asset_versions WHERE asset_id = NEW.asset_id AND version = NEW.version AND (sha256 <> NEW.sha256 OR size_bytes <> NEW.size_bytes)) BEGIN SELECT RAISE(ABORT, 'asset version is immutable'); END`,
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		for _, table := range []string{"global_assets", "global_asset_versions"} {
			for _, operation := range []string{"UPDATE", "DELETE"} {
				if err := tx.Exec("CREATE TRIGGER " + table + "_immutable_" + operation + " BEFORE " + operation + " ON " + table + " BEGIN SELECT RAISE(ABORT, 'published assets are immutable'); END").Error; err != nil {
					return err
				}
			}
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(6)").Error
	})
}
