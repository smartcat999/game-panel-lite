package instancecontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

type Postgres struct{ db *sql.DB }

func NewPostgres(db *sql.DB) *Postgres { return &Postgres{db: db} }

func (p *Postgres) InsertCreate(ctx context.Context, query persistence.DBTX, prepared PreparedCreate) error {
	configuration, err := json.Marshal(prepared.Revision.Configuration)
	if err != nil {
		return err
	}
	if _, err := query.ExecContext(ctx, `INSERT INTO logical_instances (id, workspace_id, name, game_key, desired_state, billing_state, deployment_state, stale, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, prepared.Instance.ID, prepared.Instance.WorkspaceID, prepared.Instance.Name, prepared.Instance.GameKey, prepared.Instance.DesiredState, prepared.Instance.BillingState, prepared.Instance.DeploymentState, prepared.Instance.Stale, prepared.Instance.CreatedAt); err != nil {
		return err
	}
	if _, err := query.ExecContext(ctx, `INSERT INTO instance_revisions (id, logical_instance_id, version, game_version, configuration, created_at) VALUES ($1, $2, $3, $4, $5, $6)`, prepared.Revision.ID, prepared.Revision.LogicalInstanceID, prepared.Revision.Version, prepared.Revision.GameVersion, configuration, prepared.Revision.CreatedAt); err != nil {
		return err
	}
	_, err = query.ExecContext(ctx, `INSERT INTO placements (id, logical_instance_id, region_id, version, created_at) VALUES ($1, $2, $3, $4, $5)`, prepared.Placement.ID, prepared.Placement.LogicalInstanceID, prepared.Placement.RegionID, prepared.Placement.Version, prepared.Placement.CreatedAt)
	return err
}

func (p *Postgres) Activate(ctx context.Context, query persistence.DBTX, instanceID contract.LogicalInstanceID) error {
	result, err := query.ExecContext(ctx, `UPDATE logical_instances SET billing_state = $1, deployment_state = $2 WHERE id = $3`, BillingActive, DeploymentWaitingRegion, instanceID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrInstanceMissing
	}
	return nil
}

func (p *Postgres) List(ctx context.Context, workspaceID *contract.WorkspaceID) ([]LogicalInstance, error) {
	query := `SELECT id, workspace_id, name, game_key, desired_state, billing_state, deployment_state, stale, created_at FROM logical_instances ORDER BY id LIMIT 100`
	args := []any{}
	if workspaceID != nil {
		query = `SELECT id, workspace_id, name, game_key, desired_state, billing_state, deployment_state, stale, created_at FROM logical_instances WHERE workspace_id = $1 ORDER BY id LIMIT 100`
		args = append(args, *workspaceID)
	}
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var instances []LogicalInstance
	for rows.Next() {
		var instance LogicalInstance
		if err := scanInstance(rows, &instance); err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

func (p *Postgres) Detail(ctx context.Context, query persistence.DBTX, workspaceID *contract.WorkspaceID, instanceID contract.LogicalInstanceID) (Detail, error) {
	instanceQuery := `SELECT id, workspace_id, name, game_key, desired_state, billing_state, deployment_state, stale, created_at FROM logical_instances WHERE id = $1`
	args := []any{instanceID}
	if workspaceID != nil {
		instanceQuery = `SELECT id, workspace_id, name, game_key, desired_state, billing_state, deployment_state, stale, created_at FROM logical_instances WHERE id = $1 AND workspace_id = $2`
		args = append(args, *workspaceID)
	}
	var detail Detail
	if err := scanInstance(query.QueryRowContext(ctx, instanceQuery, args...), &detail.Instance); errors.Is(err, sql.ErrNoRows) {
		return Detail{}, ErrInstanceMissing
	} else if err != nil {
		return Detail{}, err
	}
	var configuration []byte
	err := query.QueryRowContext(ctx, `SELECT id, logical_instance_id, version, game_version, configuration, created_at FROM instance_revisions WHERE logical_instance_id = $1 ORDER BY version DESC LIMIT 1`, instanceID).Scan(&detail.Revision.ID, &detail.Revision.LogicalInstanceID, &detail.Revision.Version, &detail.Revision.GameVersion, &configuration, &detail.Revision.CreatedAt)
	if err != nil {
		return Detail{}, err
	}
	if err := json.Unmarshal(configuration, &detail.Revision.Configuration); err != nil {
		return Detail{}, err
	}
	err = query.QueryRowContext(ctx, `SELECT id, logical_instance_id, region_id, version, created_at FROM placements WHERE logical_instance_id = $1 ORDER BY version DESC LIMIT 1`, instanceID).Scan(&detail.Placement.ID, &detail.Placement.LogicalInstanceID, &detail.Placement.RegionID, &detail.Placement.Version, &detail.Placement.CreatedAt)
	return detail, err
}

type rowScanner interface{ Scan(...any) error }

func scanInstance(row rowScanner, instance *LogicalInstance) error {
	return row.Scan(&instance.ID, &instance.WorkspaceID, &instance.Name, &instance.GameKey, &instance.DesiredState, &instance.BillingState, &instance.DeploymentState, &instance.Stale, &instance.CreatedAt)
}
