package store

import (
	"context"
	_ "embed"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed migrations/regional_001_ingress.sql
var regionalIngressSQL string

//go:embed migrations/regional_002_revision_fetch.sql
var regionalRevisionFetchSQL string

//go:embed migrations/regional_003_asset_preparation.sql
var regionalAssetPreparationSQL string

//go:embed migrations/regional_004_archive_uploads.sql
var regionalArchiveUploadsSQL string

//go:embed migrations/regional_005_backup_ingress.sql
var regionalBackupIngressSQL string

var ErrRegionMismatch = errors.New("regional database or event belongs to a different region")

// RegionalStore exposes no global identity, billing or instance mutation APIs.
// It must use its region's own database/schema and restricted runtime role.
type RegionalStore struct {
	db       *gorm.DB
	regionID string
}

func regionalMigrations() []sqlMigration {
	return []sqlMigration{{1, "regional_ingress", regionalIngressSQL}, {2, "regional_revision_fetch", regionalRevisionFetchSQL}, {3, "regional_asset_preparation", regionalAssetPreparationSQL}, {4, "regional_archive_uploads", regionalArchiveUploadsSQL}, {5, "regional_backup_ingress", regionalBackupIngressSQL}}
}

func validRegion(region string) bool { return region != "" && region == strings.TrimSpace(region) }

func MigrateRegionalPostgres(ctx context.Context, dsn, region string) error {
	if strings.TrimSpace(dsn) == "" || !validRegion(region) {
		return errors.New("regional database endpoint and region are required")
	}
	db, err := connectPostgres(dsn, 1)
	if err != nil {
		return err
	}
	pool, _ := db.DB()
	defer pool.Close()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := migratePostgres(ctx, tx, regionalMigrations()); err != nil {
			return err
		}
		identity := struct {
			ID       int
			RegionID string
		}{1, region}
		if err := tx.Table("regional_identity").Clauses(clause.OnConflict{DoNothing: true}).Create(&identity).Error; err != nil {
			return err
		}
		return checkRegionIdentity(tx, region)
	})
}

func OpenRegionalPostgres(dsn, region string, maxConnections int) (*RegionalStore, error) {
	if strings.TrimSpace(dsn) == "" || !validRegion(region) {
		return nil, errors.New("regional database endpoint and region are required")
	}
	db, err := connectPostgres(dsn, maxConnections)
	if err != nil {
		return nil, err
	}
	pool, _ := db.DB()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := checkPostgresSchema(ctx, db, regionalMigrations()); err != nil {
		pool.Close()
		return nil, err
	}
	if err := checkRegionIdentity(db.WithContext(ctx), region); err != nil {
		pool.Close()
		return nil, err
	}
	return &RegionalStore{db: db, regionID: region}, nil
}

func checkRegionIdentity(db *gorm.DB, region string) error {
	var identity struct{ RegionID string }
	if err := db.Table("regional_identity").Where("id = 1").Take(&identity).Error; err != nil {
		return err
	}
	if identity.RegionID != region {
		return ErrRegionMismatch
	}
	return nil
}

func (s *RegionalStore) Close() error {
	pool, err := s.db.DB()
	if err != nil {
		return err
	}
	return pool.Close()
}
