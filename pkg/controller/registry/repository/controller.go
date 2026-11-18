package repository

import (
	"context"
	"sync"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	repositorymodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/repository"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
)

const resyncBatchSize = 200

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct {
	mu           sync.Mutex
	resyncOffset int
}

func (c *Controller) Name() string  { return "60-registry-repository" }
func (c *Controller) Topic() string { return registryv1.TopicRepositories }

func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, _ *ctrlruntime.Config) ([]mq.Event, error) {
	c.mu.Lock()
	offset := c.resyncOffset
	c.mu.Unlock()

	list, err := repositorydb.NewDB(reg.Pool()).List(ctx, db.ListOptions{
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

	var events []mq.Event
	for _, item := range list.Items {
		if item == nil {
			continue
		}
		if item.Metadata.DeletionTimestamp != nil || item.Status.ObservedGeneration < item.Metadata.Generation {
			ev := mq.NewEvent(mq.EventReconcile, registryv1.GroupVersion, registryv1.KindRepository, "", item.Metadata.Name, item.Metadata.Generation, item.Metadata.ResourceVersion)
			ev.Topic = registryv1.TopicRepositories
			events = append(events, ev)
		}
	}
	return events, nil
}

func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, _ *ctrlruntime.Config, event mq.Event) error {
	if event.Name == "" {
		return nil
	}
	return repositorymodel.NewOperator(reg, bus).Reconcile(ctx, event.Name)
}
