package store

import (
	"context"
	"errors"
	"math"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RegisterAssetReplica is a trusted operator boundary. It does not provision
// storage or assert byte verification. Repeated registration preserves state.
func (s *Store) RegisterAssetReplica(ctx context.Context, replica assets.Replica) (assets.Replica, error) {
	if err := replica.ValidateRegistration(); err != nil {
		return assets.Replica{}, err
	}
	var result assets.Replica
	err := s.Transaction(ctx, func(tx *Store) error {
		if _, err := tx.GetRegion(ctx, replica.RegionID); err != nil {
			return err
		}
		var count int64
		if err := tx.db.WithContext(ctx).Table("global_asset_versions").Where("asset_id = ? AND version = ?", replica.AssetID, replica.AssetVersion).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return assets.ErrUnavailable
		}
		replica.Version = 1
		if err := tx.db.WithContext(ctx).Table("global_asset_replicas").Clauses(clause.OnConflict{DoNothing: true}).Create(&replica).Error; err != nil {
			return err
		}
		if err := tx.db.WithContext(ctx).Table("global_asset_replicas").Where("id = ?", replica.ID).Take(&result).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return assets.ErrReplicaConflict
			}
			return err
		}
		if result.AssetID != replica.AssetID || result.AssetVersion != replica.AssetVersion || result.RegionID != replica.RegionID || result.StorageID != replica.StorageID {
			return assets.ErrReplicaConflict
		}
		return nil
	})
	if err != nil {
		return assets.Replica{}, err
	}
	return result, nil
}

// SetAssetReplicaAvailable requires authenticated source-region identity and a
// verified local observation. It does not accept tenant-declared availability.
func (s *Store) SetAssetReplicaAvailable(ctx context.Context, sourceRegion, id string, version int64, available bool) error {
	if !validRegion(sourceRegion) || version < 1 || version == math.MaxInt64 {
		return assets.ErrReplicaConflict
	}
	result := s.db.WithContext(ctx).Table("global_asset_replicas").Where("id = ? AND region_id = ? AND version = ?", id, sourceRegion, version).Updates(map[string]any{"available": available, "version": version + 1})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return assets.ErrReplicaConflict
	}
	return nil
}

// ListRegionalAssetSources rechecks target-region authorization on every page.
// Source regions may differ from the deployment region. Results grant no access
// to the physical store and carry no endpoint supplied by the tenant.
func (s *Store) ListRegionalAssetSources(ctx context.Context, targetRegion string, event instances.RevisionAvailable, ref instances.AssetVersion, after string, limit int) ([]assets.Replica, error) {
	if limit < 1 || limit > 100 {
		return nil, assets.ErrInvalidVersion
	}
	var result []assets.Replica
	err := s.withRegionalRevision(ctx, targetRegion, event, func(tx *Store, snapshot regional.RevisionSnapshot) error {
		found := false
		for _, candidate := range snapshot.Revision.Specification.Assets {
			if candidate == ref {
				found = true
				break
			}
		}
		if !found {
			return ErrNotFound
		}
		if _, err := tx.resolveGlobalAssets(ctx, event.OrganizationID, []instances.AssetVersion{ref}); err != nil {
			return err
		}
		return tx.db.WithContext(ctx).Table("global_asset_replicas").Where("asset_id = ? AND asset_version = ? AND available = ? AND id > ?", ref.AssetID, ref.Version, true, after).Order("id").Limit(limit).Find(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ResolveRegionalAssetSource pins the target's authorized reference to an exact
// available replica observation in the same read snapshot. A stale directory
// page must be refreshed even if a source has become available again.
func (s *Store) ResolveRegionalAssetSource(ctx context.Context, targetRegion string, event instances.RevisionAvailable, ref instances.AssetVersion, replicaID string, replicaVersion int64) (regional.AssetSourceSnapshot, error) {
	var result regional.AssetSourceSnapshot
	err := s.withRegionalRevision(ctx, targetRegion, event, func(tx *Store, snapshot regional.RevisionSnapshot) error {
		found := false
		for _, candidate := range snapshot.Revision.Specification.Assets {
			if candidate == ref {
				found = true
				break
			}
		}
		if !found || replicaID == "" || replicaVersion < 1 {
			return ErrNotFound
		}
		manifest, err := tx.resolveGlobalAssets(ctx, event.OrganizationID, []instances.AssetVersion{ref})
		if err != nil {
			return err
		}
		var replica assets.Replica
		if err := tx.db.WithContext(ctx).Table("global_asset_replicas").Where("id = ? AND asset_id = ? AND asset_version = ? AND available = ? AND version = ?", replicaID, ref.AssetID, ref.Version, true, replicaVersion).Take(&replica).Error; err != nil {
			return err
		}
		result = regional.AssetSourceSnapshot{Event: event, Asset: manifest[0], Replica: replica}
		if result.ValidateFor(event, ref, replicaID, replicaVersion) != nil {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return regional.AssetSourceSnapshot{}, err
	}
	return result, nil
}

func migrateSQLiteAssetReplicas(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 7").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(assetReplicasSQL).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(7)").Error
	})
}
