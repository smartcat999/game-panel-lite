package instanceconfiguration

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

type PostgresStore struct{ database *sql.DB }

func NewPostgresStore(database *sql.DB) *PostgresStore { return &PostgresStore{database: database} }

func (s *PostgresStore) Create(ctx context.Context, draft Draft) error {
	values, err := json.Marshal(draft.Values)
	if err != nil {
		return err
	}
	selections, err := json.Marshal(draft.ModSelections)
	if err != nil {
		return err
	}
	errorsJSON, err := json.Marshal(draft.ValidationErrors)
	if err != nil {
		return err
	}
	_, err = s.database.ExecContext(ctx, `INSERT INTO configuration_drafts (id,workspace_id,logical_instance_id,base_revision_id,schema_version,values,mod_selections,validation_errors,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, draft.ID, draft.WorkspaceID, draft.LogicalInstanceID, draft.BaseRevisionID, draft.SchemaVersion, values, selections, errorsJSON, draft.UpdatedAt)
	return err
}

func (s *PostgresStore) Save(ctx context.Context, draft Draft) error {
	values, err := json.Marshal(draft.Values)
	if err != nil {
		return err
	}
	selections, err := json.Marshal(draft.ModSelections)
	if err != nil {
		return err
	}
	errorsJSON, err := json.Marshal(draft.ValidationErrors)
	if err != nil {
		return err
	}
	result, err := s.database.ExecContext(ctx, `UPDATE configuration_drafts SET schema_version=$4,values=$5,mod_selections=$6,validation_errors=$7,updated_at=$8 WHERE id=$1 AND workspace_id=$2 AND logical_instance_id=$3`, draft.ID, draft.WorkspaceID, draft.LogicalInstanceID, draft.SchemaVersion, values, selections, errorsJSON, draft.UpdatedAt)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *PostgresStore) ByID(ctx context.Context, workspaceID, instanceID, draftID string) (Draft, error) {
	var draft Draft
	var values, selections, validationErrors []byte
	err := s.database.QueryRowContext(ctx, `SELECT id,workspace_id,logical_instance_id,base_revision_id,schema_version,values,mod_selections,validation_errors,updated_at FROM configuration_drafts WHERE id=$1 AND workspace_id=$2 AND logical_instance_id=$3`, draftID, workspaceID, instanceID).Scan(&draft.ID, &draft.WorkspaceID, &draft.LogicalInstanceID, &draft.BaseRevisionID, &draft.SchemaVersion, &values, &selections, &validationErrors, &draft.UpdatedAt)
	if err != nil {
		return Draft{}, err
	}
	if err = json.Unmarshal(values, &draft.Values); err != nil {
		return Draft{}, err
	}
	if err = json.Unmarshal(selections, &draft.ModSelections); err != nil {
		return Draft{}, err
	}
	if err = json.Unmarshal(validationErrors, &draft.ValidationErrors); err != nil {
		return Draft{}, err
	}
	return draft, nil
}

func newDraftID() (string, error) { return persistence.NewID("drf") }
