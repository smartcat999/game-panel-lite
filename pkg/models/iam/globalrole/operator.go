package globalrole

import (
	"context"
	"fmt"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	globalroledb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrole"
	globalrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrolebinding"
)

var builtInGlobalRoles = map[string]bool{
	"platform-admin":     true,
	"node-agent":         true,
	"authenticated-user": true,
	"registry-worker":    true,
}

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.GlobalRole], error)
	Get(ctx context.Context, name string) (*iamv1.GlobalRole, error)
	Create(ctx context.Context, role *iamv1.GlobalRole) (*iamv1.GlobalRole, error)
	Update(ctx context.Context, name string, role *iamv1.GlobalRole) (*iamv1.GlobalRole, error)
	Delete(ctx context.Context, name string) error
}

type operator struct {
	db        *globalroledb.DB
	bindingDB *globalrolebindingdb.DB
	bus       mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	op := &operator{bus: bus}
	if reg != nil {
		op.db = globalroledb.NewDB(reg.Pool())
		op.bindingDB = globalrolebindingdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, eventType mq.EventType, tenant, name string, generation int64, resourceVersion string) {
	if o.bus == nil {
		return
	}
	ev := mq.NewEvent(eventType, iamv1.GroupVersion, "GlobalRole", tenant, name, generation, resourceVersion)
	ev.Topic = mq.TopicGlobalRole
	_ = o.bus.Publish(ctx, mq.TopicGlobalRole, ev)
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.GlobalRole], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, name string) (*iamv1.GlobalRole, error) {
	return o.db.Get(ctx, name)
}

func (o *operator) Create(ctx context.Context, role *iamv1.GlobalRole) (*iamv1.GlobalRole, error) {
	created, err := o.db.Create(ctx, role)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventCreated, "", created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion)
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, role *iamv1.GlobalRole) (*iamv1.GlobalRole, error) {
	role.Metadata.Name = name
	updated, err := o.db.Update(ctx, role)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventUpdated, "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	if builtInGlobalRoles[name] {
		return fmt.Errorf("%w: cannot delete built-in globalrole %q", models.ErrConflict, name)
	}
	if bindings, err := o.bindingDB.List(ctx, db.ListOptions{}); err == nil && bindings != nil {
		inUse := 0
		for _, b := range bindings.Items {
			if b != nil && b.RoleRef.Name == name {
				inUse++
			}
		}
		if inUse > 0 {
			return fmt.Errorf("%w: globalrole %q is still referenced by %d globalrolebinding(s)", models.ErrConflict, name, inUse)
		}
	}
	if err := o.db.Delete(ctx, name); err != nil {
		return err
	}
	o.publish(ctx, mq.EventDeleted, "", name, 0, "")
	return nil
}
