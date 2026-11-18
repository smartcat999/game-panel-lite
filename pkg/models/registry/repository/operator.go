package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	artifactdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/artifact"
	blobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/blob"
	pkgdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/pkg"
	policydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/policy"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
	upstreamdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/upstream"
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Repository], error)
	Get(ctx context.Context, name string) (*registryv1.Repository, error)
	Create(ctx context.Context, in *registryv1.Repository) (*registryv1.Repository, error)
	Update(ctx context.Context, name string, in *registryv1.Repository) (*registryv1.Repository, error)
	UpdateStatus(ctx context.Context, name string, in *registryv1.Repository) (*registryv1.Repository, error)
	Delete(ctx context.Context, name string) error
	Reconcile(ctx context.Context, name string) error
}

type operator struct {
	db         *repositorydb.DB
	upstreamDB *upstreamdb.DB
	pkgDB      *pkgdb.DB
	artDB      *artifactdb.DB
	blobDB     *blobdb.DB
	policyDB   *policydb.DB
	bus        mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	op := &operator{bus: bus}
	if reg != nil {
		op.db = repositorydb.NewDB(reg.Pool())
		op.upstreamDB = upstreamdb.NewDB(reg.Pool())
		op.pkgDB = pkgdb.NewDB(reg.Pool())
		op.artDB = artifactdb.NewDB(reg.Pool())
		op.blobDB = blobdb.NewDB(reg.Pool())
		op.policyDB = policydb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, evType mq.EventType, r *registryv1.Repository) {
	if o.bus == nil || r == nil {
		return
	}
	ev := mq.NewEvent(evType, registryv1.GroupVersion, registryv1.KindRepository, "", r.Metadata.Name, r.Metadata.Generation, r.Metadata.ResourceVersion)
	ev.Topic = registryv1.TopicRepositories
	_ = o.bus.Publish(ctx, registryv1.TopicRepositories, ev)
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Repository], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, name string) (*registryv1.Repository, error) {
	return o.db.Get(ctx, name)
}

func (o *operator) validateRefs(ctx context.Context, in *registryv1.Repository) error {
	if in.Spec.Mode == registryv1.RepositoryModeProxy {
		if _, err := o.upstreamDB.Get(ctx, in.Spec.UpstreamName); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return fmt.Errorf("%w: upstream %q not found", models.ErrBadRequest, in.Spec.UpstreamName)
			}
			return err
		}
	}
	if in.Spec.Mode == registryv1.RepositoryModeVirtual {
		for _, m := range in.Spec.MemberRepos {
			if m == in.Metadata.Name {
				return fmt.Errorf("%w: virtual repository cannot include itself", models.ErrBadRequest)
			}
			if _, err := o.db.Get(ctx, m); err != nil {
				if errors.Is(err, db.ErrNotFound) {
					return fmt.Errorf("%w: member repository %q not found", models.ErrBadRequest, m)
				}
				return err
			}
		}
	}
	return nil
}

func (o *operator) Create(ctx context.Context, in *registryv1.Repository) (*registryv1.Repository, error) {
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if err := o.validateRefs(ctx, &cp); err != nil {
		return nil, err
	}
	if !slices.Contains(cp.Metadata.Finalizers, registryv1.FinalizerRepositoryCleanup) {
		cp.Metadata.Finalizers = append(cp.Metadata.Finalizers, registryv1.FinalizerRepositoryCleanup)
	}
	cp.Status.Phase = registryv1.RepositoryPhaseActive
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

func (o *operator) Update(ctx context.Context, name string, in *registryv1.Repository) (*registryv1.Repository, error) {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	cp := *in
	cp.Metadata.Name = existing.Metadata.Name
	cp.Metadata.DeletionTimestamp = existing.Metadata.DeletionTimestamp
	if len(cp.Metadata.Finalizers) == 0 {
		cp.Metadata.Finalizers = append([]string(nil), existing.Metadata.Finalizers...)
	}
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if err := o.validateRefs(ctx, &cp); err != nil {
		return nil, err
	}
	updated, err := o.db.Update(ctx, &cp)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventUpdated, updated)
	return updated, nil
}

func (o *operator) UpdateStatus(ctx context.Context, name string, in *registryv1.Repository) (*registryv1.Repository, error) {
	cp := *in
	cp.Metadata.Name = name
	updated, err := o.db.UpdateStatus(ctx, &cp)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventStateChanged, updated)
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return err
	}
	if len(existing.Metadata.Finalizers) == 0 {
		if err := o.db.Delete(ctx, name); err != nil {
			return err
		}
		o.publish(ctx, mq.EventDeleted, existing)
		return nil
	}
	if existing.Metadata.DeletionTimestamp == nil {
		now := time.Now().UTC()
		existing.Metadata.DeletionTimestamp = &now
		updated, err := o.db.UpdateObject(ctx, existing)
		if err != nil {
			return err
		}
		existing = updated
	}
	o.publish(ctx, mq.EventDeleted, existing)
	return nil
}
