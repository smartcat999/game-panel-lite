package store

import (
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
	return initialize(db)
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
