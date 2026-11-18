package globalrolebinding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/models/iam/access"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	globalroledb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrole"
	globalrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrolebinding"
)

func builtInAdminName() string {
	if n := strings.TrimSpace(config.Get().Bootstrap.AdminUser); n != "" {
		return n
	}
	return "admin"
}

func (o *operator) validateReferences(ctx context.Context, binding *iamv1.GlobalRoleBinding) error {
	roleName := strings.TrimSpace(binding.RoleRef.Name)
	if roleName == "" {
		return fmt.Errorf("%w: roleRef.name is required", models.ErrBadRequest)
	}
	if _, err := o.roleDB.Get(ctx, roleName); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fmt.Errorf("%w: globalrole %q not found", models.ErrNotFound, roleName)
		}
		return err
	}
	return access.ValidateActiveUsers(ctx, o.reg, binding.Subjects)
}

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.GlobalRoleBinding], error)
	Get(ctx context.Context, name string) (*iamv1.GlobalRoleBinding, error)
	Create(ctx context.Context, binding *iamv1.GlobalRoleBinding) (*iamv1.GlobalRoleBinding, error)
	Update(ctx context.Context, name string, binding *iamv1.GlobalRoleBinding) (*iamv1.GlobalRoleBinding, error)
	Delete(ctx context.Context, name string) error
}

type operator struct {
	db     *globalrolebindingdb.DB
	roleDB *globalroledb.DB
	reg    *repo.Registry
	bus    mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	op := &operator{reg: reg, bus: bus}
	if reg != nil {
		op.db = globalrolebindingdb.NewDB(reg.Pool())
		op.roleDB = globalroledb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, eventType mq.EventType, tenant, name string, generation int64, resourceVersion string) {
	if o.bus == nil {
		return
	}
	ev := mq.NewEvent(eventType, iamv1.GroupVersion, "GlobalRoleBinding", tenant, name, generation, resourceVersion)
	ev.Topic = mq.TopicGlobalRoleBinding
	_ = o.bus.Publish(ctx, mq.TopicGlobalRoleBinding, ev)
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.GlobalRoleBinding], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, name string) (*iamv1.GlobalRoleBinding, error) {
	return o.db.Get(ctx, name)
}

func (o *operator) Create(ctx context.Context, binding *iamv1.GlobalRoleBinding) (*iamv1.GlobalRoleBinding, error) {
	binding.DefaultMeta()
	if err := o.validateReferences(ctx, binding); err != nil {
		return nil, err
	}
	created, err := o.db.Create(ctx, binding)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventCreated, "", created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion)
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, binding *iamv1.GlobalRoleBinding) (*iamv1.GlobalRoleBinding, error) {
	binding.DefaultMeta()
	current, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if (binding.Metadata.UID != "" && binding.Metadata.UID != current.Metadata.UID) ||
		(binding.Metadata.ResourceVersion != "" && binding.Metadata.ResourceVersion != current.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	binding.Metadata.Tenant = current.Metadata.Tenant
	binding.Metadata.Name = name
	binding.Metadata.UID = current.Metadata.UID
	binding.Metadata.ResourceVersion = current.Metadata.ResourceVersion
	adminName := builtInAdminName()
	if name == adminName+"-platform-admin" {
		hasAdmin := false
		for _, s := range binding.Subjects {
			if s.Kind == iamv1.SubjectKindUser && s.Name == adminName {
				hasAdmin = true
				break
			}
		}
		if !hasAdmin || binding.RoleRef.Name != "platform-admin" {
			return nil, fmt.Errorf("%w: cannot remove built-in administrator %q from %q", models.ErrConflict, adminName, name)
		}
	}
	if err := o.validateReferences(ctx, binding); err != nil {
		return nil, err
	}
	updated, err := o.db.Update(ctx, binding)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventUpdated, "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	adminBinding := builtInAdminName() + "-platform-admin"
	if name == "system-node-agents" || name == "system-authenticated-users" || name == adminBinding {
		return fmt.Errorf("%w: cannot delete built-in globalrolebinding %q", models.ErrConflict, name)
	}
	if err := o.db.Delete(ctx, name); err != nil {
		return err
	}
	o.publish(ctx, mq.EventDeleted, "", name, 0, "")
	return nil
}
