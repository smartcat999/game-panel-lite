package runtimeclass

import (
	"context"
	"fmt"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	runtimeclassdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/runtimeclass"
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.RuntimeClass], error)
	Get(ctx context.Context, name string) (*corev1.RuntimeClass, error)
	Create(ctx context.Context, rc *corev1.RuntimeClass) (*corev1.RuntimeClass, error)
	Update(ctx context.Context, name string, rc *corev1.RuntimeClass) (*corev1.RuntimeClass, error)
	Delete(ctx context.Context, name string) error
	ResolveDefault(ctx context.Context) (*corev1.RuntimeClass, error)
}

type operator struct {
	db         *runtimeclassdb.DB
	instanceDB *instancedb.DB
	bus        mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	op := &operator{bus: bus}
	if reg != nil {
		op.db = runtimeclassdb.NewDB(reg.Pool())
		op.instanceDB = instancedb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, eventType mq.EventType, tenant, name string, generation int64, resourceVersion string) {
	if o.bus == nil {
		return
	}
	ev := mq.NewEvent(eventType, corev1.GroupVersion, "RuntimeClass", tenant, name, generation, resourceVersion)
	ev.Topic = mq.TopicRuntimeClass
	_ = o.bus.Publish(ctx, mq.TopicRuntimeClass, ev)
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.RuntimeClass], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, name string) (*corev1.RuntimeClass, error) {
	return o.db.Get(ctx, name)
}

func (o *operator) clearOtherDefaults(ctx context.Context, keepName string) {
	list, err := o.db.List(ctx, db.ListOptions{})
	if err != nil || list == nil {
		return
	}
	for _, item := range list.Items {
		if item == nil || item.Metadata.Name == keepName || !item.IsDefault {
			continue
		}
		item.IsDefault = false
		if updated, err := o.db.Update(ctx, item); err == nil {
			o.publish(ctx, mq.EventUpdated, "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
		}
	}
}

func (o *operator) Create(ctx context.Context, rc *corev1.RuntimeClass) (*corev1.RuntimeClass, error) {
	created, err := o.db.Create(ctx, rc)
	if err != nil {
		return nil, err
	}
	if created.IsDefault {
		o.clearOtherDefaults(ctx, created.Metadata.Name)
	}
	o.publish(ctx, mq.EventCreated, "", created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion)
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, rc *corev1.RuntimeClass) (*corev1.RuntimeClass, error) {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing.Handler != rc.Handler {
		if err := o.rejectInUse(ctx, existing); err != nil {
			return nil, err
		}
	}
	rc.Metadata.Name = name
	updated, err := o.db.Update(ctx, rc)
	if err != nil {
		return nil, err
	}
	if updated.IsDefault {
		o.clearOtherDefaults(ctx, updated.Metadata.Name)
	}
	o.publish(ctx, mq.EventUpdated, "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
	return updated, nil
}

var builtInRuntimeClasses = map[string]bool{
	"docker": true,
	"memory": true,
}

func (o *operator) Delete(ctx context.Context, name string) error {
	if builtInRuntimeClasses[name] {
		return fmt.Errorf("%w: cannot delete built-in runtimeclass %q", models.ErrConflict, name)
	}
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return err
	}
	if err := o.rejectInUse(ctx, existing); err != nil {
		return err
	}
	if err := o.db.Delete(ctx, name); err != nil {
		return err
	}
	o.publish(ctx, mq.EventDeleted, "", name, 0, "")
	return nil
}

func (o *operator) rejectInUse(ctx context.Context, class *corev1.RuntimeClass) error {
	instances, err := o.instanceDB.List(ctx, db.ListOptions{SkipTotalCount: true})
	if err != nil {
		return err
	}
	for _, inst := range instances.Items {
		if inst == nil {
			continue
		}
		used := inst.Spec.RuntimeClassName == class.Metadata.Name
		if control := inst.Status.ResourceControl; control != nil && control.RuntimeClass != nil {
			used = used || control.RuntimeClass.Name == class.Metadata.Name || control.RuntimeClass.UID == class.Metadata.UID
		} else if inst.Spec.RuntimeClassName == "" {
			used = used || class.IsDefault
		}
		if used {
			return fmt.Errorf("%w: runtimeclass %q is still used by instance %s/%s", models.ErrConflict, class.Metadata.Name, inst.Metadata.Tenant, inst.Metadata.Name)
		}
	}
	return nil
}

func (o *operator) ResolveDefault(ctx context.Context) (*corev1.RuntimeClass, error) {
	list, err := o.db.List(ctx, db.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, rc := range list.Items {
		if rc.IsDefault {
			return rc, nil
		}
	}
	return nil, models.ErrNotFound
}
