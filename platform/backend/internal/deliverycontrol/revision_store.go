package deliverycontrol

import (
	"context"
	"encoding/json"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

func insertRevision(ctx context.Context, query persistence.DBTX, revision Revision) error {
	configuration, err := json.Marshal(revision.Configuration)
	if err != nil {
		return err
	}
	modLock, err := json.Marshal(revision.ModLock)
	if err != nil {
		return err
	}
	_, err = query.ExecContext(ctx, `INSERT INTO managed_instance_revisions (id,operation_id,workspace_id,logical_instance_id,provider_release_id,game_version,schema_version,configuration,mod_lock,apply_behavior,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, revision.ID, revision.OperationID, revision.WorkspaceID, revision.LogicalInstanceID, revision.ProviderReleaseID, revision.GameVersion, revision.SchemaVersion, configuration, modLock, revision.ApplyBehavior, revision.CreatedAt)
	return err
}

func (p *Postgres) Revision(ctx context.Context, workspaceID, instanceID, revisionID string) (Revision, error) {
	revision, err := revisionByID(ctx, p.database, revisionID)
	if err != nil || revision.WorkspaceID != workspaceID || revision.LogicalInstanceID != instanceID {
		return Revision{}, ErrNotFound
	}
	return revision, nil
}

func revisionByID(ctx context.Context, query persistence.DBTX, revisionID string) (Revision, error) {
	var revision Revision
	var configuration, modLock []byte
	err := query.QueryRowContext(ctx, `SELECT id,operation_id,workspace_id,logical_instance_id,provider_release_id,game_version,schema_version,configuration,mod_lock,apply_behavior,created_at FROM managed_instance_revisions WHERE id=$1`, revisionID).Scan(&revision.ID, &revision.OperationID, &revision.WorkspaceID, &revision.LogicalInstanceID, &revision.ProviderReleaseID, &revision.GameVersion, &revision.SchemaVersion, &configuration, &modLock, &revision.ApplyBehavior, &revision.CreatedAt)
	if err != nil {
		return Revision{}, err
	}
	if err := json.Unmarshal(configuration, &revision.Configuration); err != nil {
		return Revision{}, err
	}
	if err := json.Unmarshal(modLock, &revision.ModLock); err != nil {
		return Revision{}, err
	}
	return revision, nil
}

func revisionByOperation(ctx context.Context, query persistence.DBTX, operationID string) (Revision, error) {
	var revisionID string
	if err := query.QueryRowContext(ctx, `SELECT id FROM managed_instance_revisions WHERE operation_id=$1`, operationID).Scan(&revisionID); err != nil {
		return Revision{}, err
	}
	return revisionByID(ctx, query, revisionID)
}
