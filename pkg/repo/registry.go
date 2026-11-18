package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
)

// Config is an alias for db.Config (config.DatabaseConfig).
type Config = db.Config

func NewConfig() *Config {
	return db.NewConfig()
}

// Registry owns the underlying PostgreSQL connection pool handle.
// Operators and controllers construct their own typed domain repositories via r.Pool().
type Registry struct {
	pool *pgxpool.Pool
}

// NewPostgresRegistry constructs a Registry backed by PostgreSQL.
func NewPostgresRegistry(pool *pgxpool.Pool) *Registry {
	return &Registry{pool: pool}
}

// Open initializes the PostgreSQL connection pool, runs migrations, and returns a Registry.
func Open(ctx context.Context, cfg *Config) (*Registry, func(), error) {
	pool, closePool, err := db.OpenPool(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	return NewPostgresRegistry(pool), closePool, nil
}

// Pool returns the underlying PostgreSQL connection pool.
func (r *Registry) Pool() *pgxpool.Pool {
	if r == nil {
		return nil
	}
	return r.pool
}

// Ping verifies underlying database liveness/readiness.
func (r *Registry) Ping(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return errors.New("database registry is nil")
	}
	return r.pool.Ping(ctx)
}
