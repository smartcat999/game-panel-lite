package artifact

import (
	"context"
	"sync"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	artifactmodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/artifact"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	artifactdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/artifact"
)

const resyncBatchSize = 200

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct {
	mu           sync.Mutex
	resyncOffset int
}

func (c *Controller) Name() string  { return "61-registry-artifact" }
func (c *Controller) Topic() string { return registryv1.TopicArtifacts }

func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, _ *ctrlruntime.Config) ([]mq.Event, error) {
	c.mu.Lock()
	offset := c.resyncOffset
	c.mu.Unlock()

	list, err := artifactdb.NewDB(reg.Pool()).List(ctx, db.ListOptions{
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
		if item == nil {
			continue
		}
		needsBlobBind := item.Status.Phase == registryv1.ArtifactPhaseReady &&
			item.Status.Descriptor != nil &&
			item.Metadata.Annotations[registryv1.AnnotationBoundBlobDigest] != item.Status.Descriptor.Digest
		leaseExpired := artifactmodel.IsSyncLeaseExpired(item, now)
		if item.Metadata.DeletionTimestamp != nil || needsBlobBind || leaseExpired {
			ev := mq.NewEvent(mq.EventReconcile, registryv1.GroupVersion, registryv1.KindArtifact, "", item.Metadata.Name, item.Metadata.Generation, item.Metadata.ResourceVersion)
			ev.Topic = registryv1.TopicArtifacts
			events = append(events, ev)
		}
	}
	return events, nil
}

func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, _ *ctrlruntime.Config, event mq.Event) error {
	if event.Name == "" {
		return nil
	}
	return artifactmodel.NewOperator(reg, bus).Reconcile(ctx, event.Name)
}
