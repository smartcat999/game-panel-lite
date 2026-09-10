package deliveryworker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceaction"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceobservability"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaldelivery"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaltask"
)

type Consumer interface {
	ConsumeOne(context.Context, string, string, string, func([]byte) error) (bool, error)
}

type Global struct {
	Dispatch  messaging.Dispatcher
	Consume   Consumer
	Control   *deliverycontrol.Postgres
	Actions   *instanceaction.Postgres
	Telemetry *instanceobservability.Postgres
}

func (w Global) Tick(ctx context.Context, now time.Time) error {
	_, dispatchErr := w.Dispatch.Dispatch(ctx, messaging.MaxDispatchBatch, now)
	consumeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	_, deploymentErr := w.Consume.ConsumeOne(consumeCtx, "GAMEPANEL_GLOBAL", "global-deployment-observed", "gamepanel.global.deployment.observed.v1", func(data []byte) error {
		var envelope messaging.Envelope
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.SchemaVersion != "v1" || envelope.MessageType != "deployment.observed.v1" {
			return messaging.Permanent(errors.New("invalid deployment observation envelope"))
		}
		var observation deliverycontrol.Observation
		if err := json.Unmarshal(envelope.Payload, &observation); err != nil {
			return messaging.Permanent(err)
		}
		observation.MessageID = string(envelope.MessageID)
		_, err := w.Control.ApplyObservation(ctx, observation, now)
		if errors.Is(err, deliverycontrol.ErrInvalidCommand) || errors.Is(err, deliverycontrol.ErrNotFound) {
			return messaging.Permanent(err)
		}
		return err
	})
	var telemetryErr, backupErr, actionErr error
	if w.Telemetry != nil {
		_, telemetryErr = w.consumeTelemetry(ctx, now)
	}
	if w.Actions != nil {
		_, backupErr = w.consumeBackup(ctx, now)
		_, actionErr = w.consumeAction(ctx, now)
	}
	return errors.Join(dispatchErr, deploymentErr, telemetryErr, backupErr, actionErr)
}

func (w Global) consumeAction(ctx context.Context, now time.Time) (bool, error) {
	consumeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	return w.Consume.ConsumeOne(consumeCtx, "GAMEPANEL_GLOBAL", "global-instance-action", "gamepanel.global.instance.action.observed.v1", func(data []byte) error {
		var envelope messaging.Envelope
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.SchemaVersion != "v1" || envelope.MessageType != "instance.action.observed.v1" {
			return messaging.Permanent(errors.New("invalid action observation envelope"))
		}
		var observation instanceaction.ActionObservation
		if err := json.Unmarshal(envelope.Payload, &observation); err != nil {
			return messaging.Permanent(err)
		}
		observation.MessageID = string(envelope.MessageID)
		_, err := w.Actions.ApplyActionObservation(ctx, observation)
		if errors.Is(err, instanceaction.ErrInvalidAction) {
			return messaging.Permanent(err)
		}
		return err
	})
}

func (w Global) consumeTelemetry(ctx context.Context, now time.Time) (bool, error) {
	consumeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	return w.Consume.ConsumeOne(consumeCtx, "GAMEPANEL_GLOBAL", "global-instance-telemetry", "gamepanel.global.instance.telemetry.observed.v1", func(data []byte) error {
		var envelope messaging.Envelope
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.SchemaVersion != "v1" || envelope.MessageType != "instance.telemetry.observed.v1" {
			return messaging.Permanent(errors.New("invalid telemetry envelope"))
		}
		var observation instanceobservability.Observation
		if err := json.Unmarshal(envelope.Payload, &observation); err != nil {
			return messaging.Permanent(err)
		}
		observation.MessageID = string(envelope.MessageID)
		_, err := w.Telemetry.Handle(ctx, observation)
		if errors.Is(err, instanceobservability.ErrInvalidObservation) {
			return messaging.Permanent(err)
		}
		return err
	})
}

func (w Global) consumeBackup(ctx context.Context, now time.Time) (bool, error) {
	consumeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	return w.Consume.ConsumeOne(consumeCtx, "GAMEPANEL_GLOBAL", "global-backup-observed", "gamepanel.global.backup.observed.v1", func(data []byte) error {
		var envelope messaging.Envelope
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.SchemaVersion != "v1" || envelope.MessageType != "backup.observed.v1" {
			return messaging.Permanent(errors.New("invalid backup observation envelope"))
		}
		var observation instanceaction.BackupObservation
		if err := json.Unmarshal(envelope.Payload, &observation); err != nil {
			return messaging.Permanent(err)
		}
		observation.MessageID = string(envelope.MessageID)
		_, err := w.Actions.ApplyBackupObservation(ctx, observation)
		if errors.Is(err, instanceaction.ErrInvalidAction) {
			return messaging.Permanent(err)
		}
		return err
	})
}

type Region struct {
	RegionID string
	Dispatch messaging.Dispatcher
	Consume  Consumer
	Control  *regionaldelivery.Postgres
	Tasks    *regionaltask.Postgres
}

func (w Region) Tick(ctx context.Context, now time.Time) error {
	consumeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	_, consumeErr := w.Consume.ConsumeOne(consumeCtx, "GAMEPANEL_REGION", "region-"+w.RegionID+"-deployment", "gamepanel.region."+w.RegionID+".deployment.desired.v1", func(data []byte) error {
		var envelope messaging.Envelope
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.SchemaVersion != "v1" || envelope.MessageType != "deployment.desired.v1" {
			return messaging.Permanent(errors.New("invalid desired deployment envelope"))
		}
		var desired deliverycontrol.DesiredPayload
		if err := json.Unmarshal(envelope.Payload, &desired); err != nil {
			return messaging.Permanent(err)
		}
		_, err := w.Control.ReceiveDesired(ctx, string(envelope.MessageID), desired, now)
		if errors.Is(err, regionaldelivery.ErrInvalidDesired) || errors.Is(err, regionaldelivery.ErrWrongRegion) {
			return messaging.Permanent(err)
		}
		return err
	})
	var consoleErr, backupErr error
	if w.Tasks != nil {
		_, consoleErr = w.consumeConsole(ctx, now)
		_, backupErr = w.consumeBackup(ctx, now)
	}
	_, reconcileErr := w.Control.ReconcileOne(ctx, "region-controller-"+w.RegionID, now)
	_, dispatchErr := w.Dispatch.Dispatch(ctx, messaging.MaxDispatchBatch, now)
	return errors.Join(consumeErr, consoleErr, backupErr, reconcileErr, dispatchErr)
}

func (w Region) consumeConsole(ctx context.Context, now time.Time) (bool, error) {
	consumeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	return w.Consume.ConsumeOne(consumeCtx, "GAMEPANEL_REGION", "region-"+w.RegionID+"-console", "gamepanel.region."+w.RegionID+".console.command.requested.v1", func(data []byte) error {
		var envelope messaging.Envelope
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.SchemaVersion != "v1" || envelope.MessageType != "console.command.requested.v1" {
			return messaging.Permanent(errors.New("invalid console command envelope"))
		}
		var payload instanceaction.ConsolePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return messaging.Permanent(err)
		}
		_, err := w.Tasks.ReceiveConsole(ctx, string(envelope.MessageID), payload, now)
		if errors.Is(err, regionaltask.ErrInvalidTask) {
			return messaging.Permanent(err)
		}
		return err
	})
}

func (w Region) consumeBackup(ctx context.Context, now time.Time) (bool, error) {
	consumeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	return w.Consume.ConsumeOne(consumeCtx, "GAMEPANEL_REGION", "region-"+w.RegionID+"-backup", "gamepanel.region."+w.RegionID+".backup.requested.v1", func(data []byte) error {
		var envelope messaging.Envelope
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.SchemaVersion != "v1" || envelope.MessageType != "backup.requested.v1" {
			return messaging.Permanent(errors.New("invalid backup request envelope"))
		}
		var payload instanceaction.BackupPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return messaging.Permanent(err)
		}
		_, err := w.Tasks.ReceiveBackup(ctx, string(envelope.MessageID), payload, now)
		if errors.Is(err, regionaltask.ErrInvalidTask) {
			return messaging.Permanent(err)
		}
		return err
	})
}
