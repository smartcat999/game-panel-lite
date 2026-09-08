package store

import (
	"context"
	"database/sql"

	"gorm.io/gorm"
)

// Bound IN parameters below SQLite's conservative variable limit as well as
// PostgreSQL's limit. This is a query implementation bound, not a product quota.
const idLookupBatchSize = 500

// Related reads compose in Go from one database snapshot. Mutation methods
// still recheck authorization under their write locks before committing.
func (s *Store) readSnapshot(ctx context.Context, read func(*Store) error) error {
	options := &sql.TxOptions{ReadOnly: true}
	if s.db.Dialector.Name() == "postgres" {
		options.Isolation = sql.LevelRepeatableRead
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return read(&Store{db: tx})
	}, options)
}
