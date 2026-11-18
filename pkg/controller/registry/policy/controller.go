package policy

import (
	"context"
	"sync"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	policymodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/policy"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	policydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/policy"
)

const resyncBatchSize = 100

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct {
	mu           sync.Mutex
	resyncOffset int
}

func (c *Controller) Name() string  { return "62-registry-replication" }
func (c *Controller) Topic() string { return registryv1.TopicReplication }

func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, _ *ctrlruntime.Config) ([]mq.Event, error) {
	c.mu.Lock()
	offset := c.resyncOffset
	c.mu.Unlock()

	list, err := policydb.NewDB(reg.Pool()).List(ctx, db.ListOptions{
		Limit:          resyncBatchSize,
		Offset:         offset,
		SkipTotalCount: true,
	})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(list.Items) == resyncBatchSize {
		c.resyncOffset = offset + resyncBatchSize
	} else {
		c.resyncOffset = 0
	}
	c.mu.Unlock()

	now := time.Now().UTC()
	var events []mq.Event
	for _, item := range list.Items {
		if item == nil || !policymodel.IsDue(item, now) {
			continue
		}
		ev := mq.NewEvent(mq.EventReconcile, registryv1.GroupVersion, registryv1.KindReplicationPolicy, "", item.Metadata.Name, item.Metadata.Generation, item.Metadata.ResourceVersion)
		ev.Topic = registryv1.TopicReplication
		events = append(events, ev)
	}
	return events, nil
}

func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, _ *ctrlruntime.Config, event mq.Event) error {
	if event.Name == "" {
		return nil
	}
	return policymodel.NewOperator(reg, bus).Reconcile(ctx, event.Name)
}
