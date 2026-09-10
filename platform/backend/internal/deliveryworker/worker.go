package deliveryworker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaldelivery"
)

type Consumer interface {
	ConsumeOne(context.Context, string, string, string, func([]byte) error) (bool, error)
}

type Global struct {
	Dispatch messaging.Dispatcher
	Consume  Consumer
	Control  *deliverycontrol.Postgres
}

func (w Global) Tick(ctx context.Context, now time.Time) error {
	_, dispatchErr := w.Dispatch.Dispatch(ctx, messaging.MaxDispatchBatch, now)
	consumeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	_, consumeErr := w.Consume.ConsumeOne(consumeCtx, "GAMEPANEL_GLOBAL", "global-control-plane", "gamepanel.global.deployment.observed.v1", func(data []byte) error {
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
	return errors.Join(dispatchErr, consumeErr)
}

type Region struct {
	RegionID string
	Dispatch messaging.Dispatcher
	Consume  Consumer
	Control  *regionaldelivery.Postgres
}

func (w Region) Tick(ctx context.Context, now time.Time) error {
	consumeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	_, consumeErr := w.Consume.ConsumeOne(consumeCtx, "GAMEPANEL_REGION", "region-controller-"+w.RegionID, "gamepanel.region."+w.RegionID+".deployment.desired.v1", func(data []byte) error {
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
	_, reconcileErr := w.Control.ReconcileOne(ctx, "region-controller-"+w.RegionID, now)
	_, dispatchErr := w.Dispatch.Dispatch(ctx, messaging.MaxDispatchBatch, now)
	return errors.Join(consumeErr, reconcileErr, dispatchErr)
}
