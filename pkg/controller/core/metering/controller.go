package metering

import (
	"context"
	"time"

	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	alertmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert"
	meteringmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/metering"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct{}

func (c *Controller) Name() string  { return "06-metering" }
func (c *Controller) Topic() string { return mq.TopicMetering }

func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, _ *ctrlruntime.Config) ([]mq.Event, error) {
	meteringmodel.RefreshMeteringSnapshots(ctx, reg)
	alertmodel.RefreshAlertSnapshotAsync(reg)
	return nil, nil
}

// Reconcile consumes CNCF CloudEvents 1.0 usage metering transitions from mq.TopicMetering ("billing.v1.metering")
// and invalidates/refreshes tenant metering snapshots asynchronously.
func (c *Controller) Reconcile(_ context.Context, reg *repo.Registry, _ mq.Interface, _ *ctrlruntime.Config, event mq.Event) error {
	if event.Tenant == "" {
		return nil
	}
	ce, err := mq.ParseMeteringCloudEvent(event)
	if err != nil {
		return err
	}
	event.ID = ce.ID()
	event.Type = mq.EventType(ce.Type())
	var data map[string]string
	if err := ce.DataAs(&data); err != nil {
		return err
	}
	attrs := make(map[string]string, len(event.Attributes)+len(data))
	for key, value := range event.Attributes {
		attrs[key] = value
	}
	for key, value := range data {
		attrs[key] = value
	}
	event.Attributes = attrs
	ts := event.Timestamp.UTC()
	if ts.IsZero() {
		ts = ce.Time().UTC()
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	event.Timestamp = ts
	meteringmodel.RecordCloudEvent(event, reg)
	meteringmodel.RefreshTenantMeteringSnapshotsAsync(reg, event.Tenant)
	return nil
}
