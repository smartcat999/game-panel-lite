package regionaldelivery

import (
	"context"
	"database/sql"
	"errors"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceobservability"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
)

func (p *Postgres) Append(ctx context.Context, observation instanceobservability.Observation) error {
	if observation.MessageID == "" || observation.LogicalInstanceID == "" || observation.RuntimeAttemptID == "" || observation.FencingToken < 1 || observation.Sequence < 1 {
		return ErrInvalidDesired
	}
	tx, err := p.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	state, err := stateByInstance(ctx, tx, observation.LogicalInstanceID, true)
	if err != nil {
		return err
	}
	if state.WorkspaceID != observation.WorkspaceID || state.RegionID != observation.RegionID || state.ID != observation.RegionalDeploymentID || state.FencingToken != observation.FencingToken {
		return ErrFencingToken
	}
	if observation.Sequence <= state.TelemetrySequence {
		return nil
	}
	message := messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(observation.MessageID), MessageType: "instance.telemetry.observed.v1", IdempotencyKey: contract.IdempotencyKey("telemetry:" + observation.MessageID), Payload: observation, CreatedAt: observation.ObservedAt}
	if err := messaging.NewPostgres(p.database).InsertOutboxTo(ctx, tx, messaging.RegionOutbox, message); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE regional_delivery_states SET telemetry_sequence=$2 WHERE id=$1 AND fencing_token=$3 AND telemetry_sequence<$2`, state.ID, observation.Sequence, observation.FencingToken)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("telemetry sequence was not advanced")
	}
	return tx.Commit()
}
