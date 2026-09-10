package regiondirectory

import (
	"context"
	"database/sql"
	"errors"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

type Postgres struct{ db *sql.DB }

func NewPostgres(db *sql.DB) *Postgres { return &Postgres{db: db} }

func (p *Postgres) List(ctx context.Context) ([]Region, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, code, name, available FROM regions ORDER BY code LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var regions []Region
	for rows.Next() {
		var region Region
		if err := rows.Scan(&region.ID, &region.Code, &region.Name, &region.Available); err != nil {
			return nil, err
		}
		regions = append(regions, region)
	}
	return regions, rows.Err()
}

func (p *Postgres) RequireAvailable(ctx context.Context, query persistence.DBTX, regionID contract.RegionID) (Region, error) {
	var region Region
	err := query.QueryRowContext(ctx, `SELECT id, code, name, available FROM regions WHERE id = $1`, regionID).Scan(&region.ID, &region.Code, &region.Name, &region.Available)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !region.Available {
		return Region{}, ErrRegionUnavailable
	}
	return region, err
}
