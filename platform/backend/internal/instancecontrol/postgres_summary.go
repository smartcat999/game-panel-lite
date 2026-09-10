package instancecontrol

import (
	"context"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

func (p *Postgres) ApplyDeploymentSummary(ctx context.Context, query persistence.DBTX, summary DeploymentSummary) (bool, error) {
	result, err := query.ExecContext(ctx, `INSERT INTO deployment_summaries (logical_instance_id, regional_deployment_id, region_id, sequence, observed_state, observed_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (logical_instance_id) DO UPDATE SET regional_deployment_id = EXCLUDED.regional_deployment_id, region_id = EXCLUDED.region_id, sequence = EXCLUDED.sequence, observed_state = EXCLUDED.observed_state, observed_at = EXCLUDED.observed_at WHERE deployment_summaries.sequence < EXCLUDED.sequence`, summary.LogicalInstanceID, summary.RegionalDeploymentID, summary.RegionID, summary.Sequence, summary.ObservedState, summary.ObservedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (p *Postgres) DeploymentSummaries(ctx context.Context, instanceIDs []contract.LogicalInstanceID) ([]DeploymentSummary, error) {
	if len(instanceIDs) == 0 {
		return nil, nil
	}
	if len(instanceIDs) > persistence.BatchSize {
		instanceIDs = instanceIDs[:persistence.BatchSize]
	}
	rows, err := p.db.QueryContext(ctx, `SELECT logical_instance_id, regional_deployment_id, region_id, sequence, observed_state, observed_at FROM deployment_summaries WHERE logical_instance_id = ANY($1) ORDER BY logical_instance_id LIMIT 100`, instanceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summaries []DeploymentSummary
	for rows.Next() {
		var summary DeploymentSummary
		if err := rows.Scan(&summary.LogicalInstanceID, &summary.RegionalDeploymentID, &summary.RegionID, &summary.Sequence, &summary.ObservedState, &summary.ObservedAt); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}
