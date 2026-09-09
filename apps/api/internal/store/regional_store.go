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

//go:embed migrations/regional_006_backup_result_publication.sql
var regionalBackupResultPublicationSQL string

//go:embed migrations/regional_007_backup_preparation.sql
var regionalBackupPreparationSQL string

//go:embed migrations/regional_008_deployments.sql
var regionalDeploymentsSQL string

//go:embed migrations/regional_009_nodes.sql
var regionalNodesSQL string

//go:embed migrations/regional_010_node_sessions.sql
var regionalNodeSessionsSQL string

//go:embed migrations/regional_011_allocations.sql
var regionalAllocationsSQL string

//go:embed migrations/regional_012_ports.sql
var regionalPortsSQL string

//go:embed migrations/regional_013_scheduling.sql
var regionalSchedulingSQL string

//go:embed migrations/regional_014_node_access.sql
var regionalNodeAccessSQL string

//go:embed migrations/regional_015_node_tasks.sql
var regionalNodeTasksSQL string

//go:embed migrations/regional_016_status_publication.sql
var regionalStatusPublicationSQL string

//go:embed migrations/regional_017_execution_authority.sql
var regionalExecutionAuthoritySQL string

var ErrRegionMismatch = errors.New("regional database or event belongs to a different region")

// RegionalStore exposes no global identity, billing or instance mutation APIs.
// It must use its region's own database/schema and restricted runtime role.
type RegionalStore struct {
	db       *gorm.DB
	regionID string
}

func regionalMigrations() []sqlMigration {
	return []sqlMigration{{1, "regional_ingress", regionalIngressSQL}, {2, "regional_revision_fetch", regionalRevisionFetchSQL}, {3, "regional_asset_preparation", regionalAssetPreparationSQL}, {4, "regional_archive_uploads", regionalArchiveUploadsSQL}, {5, "regional_backup_ingress", regionalBackupIngressSQL}, {6, "regional_backup_result_publication", regionalBackupResultPublicationSQL}, {7, "regional_backup_preparation", regionalBackupPreparationSQL}, {8, "regional_deployments", regionalDeploymentsSQL}, {9, "regional_nodes", regionalNodesSQL}, {10, "regional_node_sessions", regionalNodeSessionsSQL}, {11, "regional_allocations", regionalAllocationsSQL}, {12, "regional_ports", regionalPortsSQL}, {13, "regional_scheduling", regionalSchedulingSQL}, {14, "regional_node_access", regionalNodeAccessSQL}, {15, "regional_node_tasks", regionalNodeTasksSQL}, {16, "regional_status_publication", regionalStatusPublicationSQL}, {17, "regional_execution_authority", regionalExecutionAuthoritySQL}}
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

// RegionID is the immutable identity verified when this regional database opens.
func (s *RegionalStore) RegionID() string { return s.regionID }
