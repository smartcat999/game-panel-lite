package tenantrolebinding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/models/iam/access"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	tenantroledb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/tenantrole"
	tenantrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/tenantrolebinding"
)

func (o *operator) validateReferences(ctx context.Context, parent *corev1.Tenant, binding *iamv1.TenantRoleBinding) error {
	tenant := parent.Metadata.Name
	roleName := strings.TrimSpace(binding.RoleRef.Name)
	if roleName == "" {
		return fmt.Errorf("%w: roleRef.name is required", models.ErrBadRequest)
	}
	if _, err := o.roleDB.Get(ctx, tenant, roleName); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fmt.Errorf("%w: tenantrole %q not found in tenant %q", models.ErrNotFound, roleName, tenant)
		}
		return err
	}
	if err := access.ValidateActiveUsers(ctx, o.reg, binding.Subjects); err != nil {
		return err
	}
	return nil
}

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.TenantRoleBinding], error)
	Get(ctx context.Context, tenant, name string) (*iamv1.TenantRoleBinding, error)
	Create(ctx context.Context, tenant string, binding *iamv1.TenantRoleBinding) (*iamv1.TenantRoleBinding, error)
	Update(ctx context.Context, tenant, name string, binding *iamv1.TenantRoleBinding) (*iamv1.TenantRoleBinding, error)
	Delete(ctx context.Context, tenant, name string) error
}

type operator struct {
	db     *tenantrolebindingdb.DB
	roleDB *tenantroledb.DB
	reg    *repo.Registry
	bus    mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	op := &operator{reg: reg, bus: bus}
	if reg != nil {
		op.db = tenantrolebindingdb.NewDB(reg.Pool())
		op.roleDB = tenantroledb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, eventType mq.EventType, tenant, name string, generation int64, resourceVersion string) {
	if o.bus == nil {
		return
	}
	ev := mq.NewEvent(eventType, iamv1.GroupVersion, "TenantRoleBinding", tenant, name, generation, resourceVersion)
	ev.Topic = mq.TopicTenantRoleBinding
	_ = o.bus.Publish(ctx, mq.TopicTenantRoleBinding, ev)
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.TenantRoleBinding], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, tenant, name string) (*iamv1.TenantRoleBinding, error) {
	return o.db.Get(ctx, tenant, name)
}

func (o *operator) Create(ctx context.Context, tenant string, binding *iamv1.TenantRoleBinding) (*iamv1.TenantRoleBinding, error) {
	binding.DefaultMeta()
	parent, err := access.ActiveTenant(ctx, o.reg, tenant)
	if err != nil {
		return nil, err
	}
	binding.Metadata.Tenant = tenant
	if err := o.validateReferences(ctx, parent, binding); err != nil {
		return nil, err
	}
	created, err := o.db.Create(ctx, binding)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventCreated, created.Metadata.Tenant, created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion)
	return created, nil
}

func (o *operator) Update(ctx context.Context, tenant, name string, binding *iamv1.TenantRoleBinding) (*iamv1.TenantRoleBinding, error) {
	binding.DefaultMeta()
	parent, err := access.ActiveTenant(ctx, o.reg, tenant)
	if err != nil {
		return nil, err
	}
	current, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if (binding.Metadata.UID != "" && binding.Metadata.UID != current.Metadata.UID) ||
		(binding.Metadata.ResourceVersion != "" && binding.Metadata.ResourceVersion != current.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	binding.Metadata.Tenant = tenant
	binding.Metadata.Name = name
	binding.Metadata.UID = current.Metadata.UID
	binding.Metadata.ResourceVersion = current.Metadata.ResourceVersion
	if err := o.validateReferences(ctx, parent, binding); err != nil {
		return nil, err
	}
	updated, err := o.db.Update(ctx, binding)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventUpdated, updated.Metadata.Tenant, updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, tenant, name string) error {
	if name == "owner-tenant-admin" {
		return fmt.Errorf("%w: cannot delete built-in owner binding %q", models.ErrConflict, name)
	}
	if err := o.db.Delete(ctx, tenant, name); err != nil {
		return err
	}
	o.publish(ctx, mq.EventDeleted, tenant, name, 0, "")
	return nil
}
