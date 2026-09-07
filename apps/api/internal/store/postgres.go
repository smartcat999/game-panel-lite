package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenConfigured retains SQLite for self-hosted deployments. PostgreSQL is
// selected explicitly by DSN; credentials must be supplied outside source control.
func OpenConfigured(path, dsn string, maxConnections int) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return Open(path)
	}
	db, err := connectPostgres(dsn, maxConnections)
	if err != nil {
		return nil, err
	}
	pool, _ := db.DB()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := checkPostgresSchema(ctx, db, postgresMigrations()); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return &Store{db: db, activitySubscribers: map[uint64]activitySubscriber{}}, nil
}

// MigratePostgres is an administrative operation for the deployment job, not
// API startup. Credentials must grant DDL rights for the selected schema.
func MigratePostgres(ctx context.Context, dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("PostgreSQL DSN is required for migration")
	}
	db, err := connectPostgres(dsn, 1)
	if err != nil {
		return err
	}
	pool, _ := db.DB()
	defer pool.Close()
	return migratePostgres(ctx, db, postgresMigrations())
}

func connectPostgres(dsn string, maxConnections int) (*gorm.DB, error) {
	if maxConnections == 0 {
		maxConnections = 20
	}
	if maxConnections < 1 {
		return nil, fmt.Errorf("database maximum connections must be positive")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		// Driver parse/connection errors can include the DSN and its credentials.
		return nil, fmt.Errorf("PostgreSQL connection failed; check database configuration and availability")
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(maxConnections)
	pool.SetMaxIdleConns(maxConnections)
	pool.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}

func (s *Store) Close() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

// SQLite preserves the legacy insertion-order tie break. PostgreSQL has no
// rowid; equal timestamps are ordered deterministically by stable record ID.
func (s *Store) creationOrder(descending bool) string {
	key := "id"
	if s.db.Dialector.Name() == "sqlite" {
		key = "rowid"
	}
	direction := "asc"
	if descending {
		direction = "desc"
	}
	return "created_at " + direction + ", " + key + " " + direction
}
