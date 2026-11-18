package policy

import (
	"context"
	"errors"
	"fmt"
	"slices"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	artifactdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/artifact"
	pkgdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/pkg"
	policydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/policy"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
	upstreamdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/upstream"
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.ReplicationPolicy], error)
	Get(ctx context.Context, name string) (*registryv1.ReplicationPolicy, error)
	Create(ctx context.Context, in *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error)
	Update(ctx context.Context, name string, in *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error)
	Delete(ctx context.Context, name string) error
	Reconcile(ctx context.Context, name string) error
}

type operator struct {
	db         *policydb.DB
	repoDB     *repositorydb.DB
	upstreamDB *upstreamdb.DB
	pkgDB      *pkgdb.DB
	artDB      *artifactdb.DB
	cache      cache.Interface
	bus        mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface, caches ...cache.Interface) Operator {
	op := &operator{bus: bus}
	if len(caches) > 0 && caches[0] != nil {
		op.cache = caches[0]
	} else {
		op.cache = cache.Default()
	}
	if reg != nil {
		op.db = policydb.NewDB(reg.Pool())
		op.repoDB = repositorydb.NewDB(reg.Pool())
		op.upstreamDB = upstreamdb.NewDB(reg.Pool())
		op.pkgDB = pkgdb.NewDB(reg.Pool())
		op.artDB = artifactdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, evType mq.EventType, p *registryv1.ReplicationPolicy) {
	if o.bus == nil || p == nil {
		return
	}
	ev := mq.NewEvent(evType, registryv1.GroupVersion, registryv1.KindReplicationPolicy, "", p.Metadata.Name, p.Metadata.Generation, p.Metadata.ResourceVersion)
	ev.Topic = registryv1.TopicReplication
	_ = o.bus.Publish(ctx, registryv1.TopicReplication, ev)
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.ReplicationPolicy], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, name string) (*registryv1.ReplicationPolicy, error) {
	return o.db.Get(ctx, name)
}

func (o *operator) Create(ctx context.Context, in *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error) {
	cp := *in
	cp.DefaultMeta()
	if _, err := o.repoDB.Get(ctx, cp.Repository); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, fmt.Errorf("%w: repository %q not found", models.ErrBadRequest, cp.Repository)
		}
		return nil, err
	}
	created, err := o.db.Create(ctx, &cp)
	if err != nil {
		if errors.Is(err, db.ErrAlreadyExists) || errors.Is(err, db.ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	o.publish(ctx, mq.EventCreated, created)
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, in *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error) {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	cp := *in
	cp.Metadata.Name = existing.Metadata.Name
	cp.DefaultMeta()
	specChanged := existing.Repository != cp.Repository ||
		existing.Enabled != cp.Enabled ||
		existing.Mode != cp.Mode ||
		existing.IntervalMinutes != cp.IntervalMinutes ||
		existing.TopN != cp.TopN ||
		existing.GameVersion != cp.GameVersion ||
		!slices.Equal(existing.Packages, cp.Packages)
	if specChanged {
		cp.LastRunAt = nil
		cp.LastMessage = ""
	} else {
		cp.LastRunAt = existing.LastRunAt
		cp.LastMessage = existing.LastMessage
	}
	if _, err := o.repoDB.Get(ctx, cp.Repository); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, fmt.Errorf("%w: repository %q not found", models.ErrBadRequest, cp.Repository)
		}
		return nil, err
	}
	updated, err := o.db.Update(ctx, &cp)
	if err != nil {
		if errors.Is(err, db.ErrConflict) || errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	o.publish(ctx, mq.EventUpdated, updated)
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return err
	}
	if err := o.db.Delete(ctx, name); err != nil {
		return err
	}
	o.publish(ctx, mq.EventDeleted, existing)
	return nil
}
