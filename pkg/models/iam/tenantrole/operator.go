package tenantrole

import (
	"context"
	"errors"
	"fmt"
	"reflect"

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

var builtInTenantRoles = map[string]bool{
	"tenant-admin":      true,
	"instance-operator": true,
	"tenant-viewer":     true,
}

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.TenantRole], error)
	Get(ctx context.Context, tenant, name string) (*iamv1.TenantRole, error)
	Create(ctx context.Context, tenant string, role *iamv1.TenantRole) (*iamv1.TenantRole, error)
	Update(ctx context.Context, tenant, name string, role *iamv1.TenantRole) (*iamv1.TenantRole, error)
	Delete(ctx context.Context, tenant, name string) error
}

type operator struct {
	db        *tenantroledb.DB
	bindingDB *tenantrolebindingdb.DB
	reg       *repo.Registry
	bus       mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	op := &operator{reg: reg, bus: bus}
	if reg != nil {
		op.db = tenantroledb.NewDB(reg.Pool())
		op.bindingDB = tenantrolebindingdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, eventType mq.EventType, tenant, name string, generation int64, resourceVersion string) {
	if o.bus == nil {
		return
	}
	ev := mq.NewEvent(eventType, iamv1.GroupVersion, "TenantRole", tenant, name, generation, resourceVersion)
	ev.Topic = mq.TopicTenantRole
	_ = o.bus.Publish(ctx, mq.TopicTenantRole, ev)
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.TenantRole], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, tenant, name string) (*iamv1.TenantRole, error) {
	return o.db.Get(ctx, tenant, name)
}

func (o *operator) Create(ctx context.Context, tenant string, role *iamv1.TenantRole) (*iamv1.TenantRole, error) {
	if _, err := access.ActiveTenant(ctx, o.reg, tenant); err != nil {
		return nil, err
	}
	role.Metadata.Tenant = tenant
	created, err := o.db.Create(ctx, role)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventCreated, created.Metadata.Tenant, created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion)
	return created, nil
}

func (o *operator) Update(ctx context.Context, tenant, name string, role *iamv1.TenantRole) (*iamv1.TenantRole, error) {
	if _, err := access.ActiveTenant(ctx, o.reg, tenant); err != nil {
		return nil, err
	}
	current, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if (role.Metadata.UID != "" && role.Metadata.UID != current.Metadata.UID) ||
		(role.Metadata.ResourceVersion != "" && role.Metadata.ResourceVersion != current.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	role.Metadata.Tenant = tenant
	role.Metadata.Name = name
	role.Metadata.UID = current.Metadata.UID
	role.Metadata.ResourceVersion = current.Metadata.ResourceVersion
	updated, err := o.db.Update(ctx, role)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventUpdated, updated.Metadata.Tenant, updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, tenant, name string) error {
	if builtInTenantRoles[name] {
		return fmt.Errorf("%w: cannot delete built-in tenantrole %q", models.ErrConflict, name)
	}
	if bindings, err := o.bindingDB.List(ctx, db.ListOptions{Tenant: tenant}); err == nil && bindings != nil {
		inUse := 0
		for _, b := range bindings.Items {
			if b != nil && b.RoleRef.Name == name {
				inUse++
			}
		}
		if inUse > 0 {
			return fmt.Errorf("%w: tenantrole %q is still referenced by %d tenantrolebinding(s)", models.ErrConflict, name, inUse)
		}
	}
	if err := o.db.Delete(ctx, tenant, name); err != nil {
		return err
	}
	o.publish(ctx, mq.EventDeleted, tenant, name, 0, "")
	return nil
}

// EnsureBuiltInRoles seeds default TenantRoles (tenant-admin, instance-operator, tenant-viewer)
// and binds the tenant owner to tenant-admin.
func EnsureBuiltInRoles(ctx context.Context, reg *repo.Registry, tenantName, ownerUser string) error {
	parent, err := access.ActiveTenant(ctx, reg, tenantName)
	if err != nil {
		return err
	}
	if ownerUser != "" && ownerUser != parent.Spec.Owner {
		return fmt.Errorf("%w: tenant owner changed before role seeding", models.ErrConflict)
	}
	roleDB := tenantroledb.NewDB(reg.Pool())
	bindingDB := tenantrolebindingdb.NewDB(reg.Pool())
	for _, r := range defaultBuiltInTenantRoles(tenantName) {
		if err := ensureSingleBuiltInRole(ctx, roleDB, tenantName, r); err != nil {
			return err
		}
	}
	if ownerUser != "" {
		bindName := ownerUser + "-tenant-admin"
		if _, err := bindingDB.Get(ctx, tenantName, bindName); errors.Is(err, db.ErrNotFound) {
			b := &iamv1.TenantRoleBinding{
				Metadata: metav1.ObjectMeta{Name: bindName, Tenant: tenantName},
				RoleRef:  iamv1.RoleRef{Name: "tenant-admin"},
				Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: ownerUser}},
			}
			if err := access.ValidateActiveUsers(ctx, reg, b.Subjects); err != nil {
				return err
			}
			if _, err := bindingDB.Create(ctx, b); err != nil && !errors.Is(err, db.ErrAlreadyExists) {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func ensureSingleBuiltInRole(ctx context.Context, roleDB *tenantroledb.DB, tenantName string, desired *iamv1.TenantRole) error {
	existing, err := roleDB.Get(ctx, tenantName, desired.Metadata.Name)
	if errors.Is(err, db.ErrNotFound) {
		if _, err := roleDB.Create(ctx, desired); err != nil && !errors.Is(err, db.ErrAlreadyExists) {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	if existing != nil && (!reflect.DeepEqual(existing.Rules, desired.Rules) || existing.Description != desired.Description) {
		existing.Rules = desired.Rules
		existing.Description = desired.Description
		_, _ = roleDB.Update(ctx, existing)
	}
	return nil
}

func defaultBuiltInTenantRoles(tenantName string) []*iamv1.TenantRole {
	return []*iamv1.TenantRole{
		{
			Metadata:    metav1.ObjectMeta{Name: "tenant-admin", Tenant: tenantName},
			Description: "Full administrative access within the tenant",
			Rules:       []iamv1.PolicyRule{{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{iamv1.VerbAll}}},
		},
		{
			Metadata:    metav1.ObjectMeta{Name: "instance-operator", Tenant: tenantName},
			Description: "View and operate existing instances (state, console/exec, files, backups, addons, schedules, config) without create/delete or member management access",
			Rules: []iamv1.PolicyRule{
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"tenants", "instances", "instances/*"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch}},
				{APIGroups: []string{iamv1.GroupName}, Resources: []string{"tenantroles", "tenantrolebindings"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch}},
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"instances"}, Verbs: []string{iamv1.VerbUpdate, iamv1.VerbPatch}},
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"instances/state", "instances/exec", "instances/files"}, Verbs: []string{iamv1.VerbCreate, iamv1.VerbUpdate, iamv1.VerbPatch}},
			},
		},
		{
			Metadata:    metav1.ObjectMeta{Name: "tenant-viewer", Tenant: tenantName},
			Description: "Read-only access to tenant resources",
			Rules:       []iamv1.PolicyRule{{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch}}},
		},
	}
}
