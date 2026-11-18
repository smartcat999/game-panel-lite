package resourceplan

import (
	"context"
	"errors"
	"fmt"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	plandb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/resourceplan"
)

// Operator provides validated CRUD operations for cluster-scoped ResourcePlans.
type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.ResourcePlan], error)
	Get(ctx context.Context, name string) (*corev1.ResourcePlan, error)
	Create(ctx context.Context, in *corev1.ResourcePlan) (*corev1.ResourcePlan, error)
	Update(ctx context.Context, name string, in *corev1.ResourcePlan) (*corev1.ResourcePlan, error)
	Delete(ctx context.Context, name string) error
}

type operator struct {
	db  *plandb.DB
	bus mq.Interface
}

// NewOperator constructs a ResourcePlan business operator.
func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	op := &operator{bus: bus}
	if reg != nil {
		op.db = plandb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, eventType mq.EventType, name string, generation int64, rv string) {
	if o.bus == nil {
		return
	}
	ev := mq.NewEvent(eventType, corev1.GroupVersion, corev1.KindResourcePlan, "", name, generation, rv)
	ev.Topic = mq.TopicResourcePlan
	_ = o.bus.Publish(ctx, mq.TopicResourcePlan, ev)
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.ResourcePlan], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, name string) (*corev1.ResourcePlan, error) {
	return o.db.Get(ctx, name)
}

func (o *operator) Create(ctx context.Context, in *corev1.ResourcePlan) (*corev1.ResourcePlan, error) {
	if in == nil {
		return nil, fmt.Errorf("%w: resourceplan payload is required", models.ErrBadRequest)
	}
	in.DefaultMeta()
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	created, err := o.db.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventCreated, created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion)
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, in *corev1.ResourcePlan) (*corev1.ResourcePlan, error) {
	if in == nil {
		return nil, fmt.Errorf("%w: resourceplan payload is required", models.ErrBadRequest)
	}
	in.Metadata.Name = name
	in.DefaultMeta()
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	current, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if (in.Metadata.UID != "" && in.Metadata.UID != current.Metadata.UID) ||
		(in.Metadata.ResourceVersion != "" && in.Metadata.ResourceVersion != current.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	in.Metadata.UID = current.Metadata.UID
	in.Metadata.ResourceVersion = current.Metadata.ResourceVersion
	in.Metadata.CreationTimestamp = current.Metadata.CreationTimestamp
	updated, err := o.db.Update(ctx, in)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventUpdated, updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	current, err := o.db.Get(ctx, name)
	if err != nil {
		return err
	}
	if err := o.db.Delete(ctx, name, current.Metadata.UID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.ErrNotFound
		}
		return err
	}
	o.publish(ctx, mq.EventDeleted, name, current.Metadata.Generation, current.Metadata.ResourceVersion)
	return nil
}
