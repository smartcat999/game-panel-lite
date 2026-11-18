package tenant

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/models/iam/access"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	tenantrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/tenantrolebinding"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
)

func builtInAdminName() string {
	if n := strings.TrimSpace(config.Get().Bootstrap.AdminUser); n != "" {
		return n
	}
	return "admin"
}

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Tenant], error)
	Get(ctx context.Context, name string) (*corev1.Tenant, error)
	CanView(ctx context.Context, name string, hint *corev1.Tenant, deletedOwner string) bool
	Create(ctx context.Context, defaultOwner string, tenant *corev1.Tenant) (*corev1.Tenant, error)
	Update(ctx context.Context, name string, tenant *corev1.Tenant) (*corev1.Tenant, error)
	UpdateStatus(ctx context.Context, name string, tenant *corev1.Tenant) (*corev1.Tenant, error)
	Delete(ctx context.Context, name string) error
}

type operator struct {
	db      *tenantdb.DB
	userDB  *userdb.DB
	trbDB   *tenantrolebindingdb.DB
	quotaDB *quotadb.DB
	reg     *repo.Registry
	mq      mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	op := &operator{reg: reg, mq: bus}
	if reg != nil {
		pool := reg.Pool()
		op.db = tenantdb.NewDB(pool)
		op.userDB = userdb.NewDB(pool)
		op.trbDB = tenantrolebindingdb.NewDB(pool)
		op.quotaDB = quotadb.NewDB(pool)
	}
	return op
}

func emptyTenantList() *metav1.List[*corev1.Tenant] {
	return &metav1.List[*corev1.Tenant]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "TenantList"},
		Metadata: metav1.ListMeta{TotalCount: 0},
		Items:    []*corev1.Tenant{},
	}
}

func nonNilTenantLabels(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Tenant], error) {
	u, hasUser := iamv1.UserFrom(ctx)
	if hasUser && (u.IsAnonymous() || u.Kind == iamv1.SubjectKindNode) {
		return emptyTenantList(), nil
	}
	if !hasUser || u.Kind == iamv1.SubjectKindSystem || o.canListAllTenants(ctx, u) {
		return o.listWithReserved(ctx, opts)
	}

	// Non-admin user: resolve visible tenant names BEFORE storage pagination so pages are never empty
	// and TotalCount / Continue cursors reflect only accessible workspaces.
	visibleSet := make(map[string]struct{})
	if owned, err := o.db.List(ctx, db.ListOptions{
		FieldSelector: map[string]string{"spec.owner": u.Name},
	}); err == nil && owned != nil {
		for _, t := range owned.Items {
			if t != nil && t.Metadata.Name != "" {
				visibleSet[t.Metadata.Name] = struct{}{}
			}
		}
	}
	if o.trbDB != nil {
		if trbs, err := o.trbDB.List(ctx, db.ListOptions{
			FieldSelector: map[string]string{"subject": access.SubjectFilter(u)},
		}); err == nil && trbs != nil {
			for _, b := range trbs.Items {
				if b != nil && b.Metadata.Tenant != "" && access.SubjectMatches(b.Subjects, u) {
					visibleSet[b.Metadata.Tenant] = struct{}{}
				}
			}
		}
	}
	if len(visibleSet) == 0 {
		return emptyTenantList(), nil
	}

	visibleNames := make([]string, 0, len(visibleSet))
	for name := range visibleSet {
		visibleNames = append(visibleNames, name)
	}
	scopedOpts := opts
	scopedSelector := make(map[string]string, len(opts.FieldSelector)+1)
	for k, v := range opts.FieldSelector {
		scopedSelector[k] = v
	}
	scopedSelector["metadata.name.in"] = strings.Join(visibleNames, ",")
	scopedOpts.FieldSelector = scopedSelector
	return o.listWithReserved(ctx, scopedOpts)
}

func (o *operator) listWithReserved(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Tenant], error) {
	list, err := o.db.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	for _, tenant := range list.Items {
		if tenant != nil {
			if err := o.projectReserved(ctx, tenant); err != nil {
				return nil, err
			}
		}
	}
	return list, nil
}

func (o *operator) projectReserved(ctx context.Context, tenant *corev1.Tenant) error {
	account, err := o.quotaDB.GetAccount(ctx, tenant.Metadata.UID)
	if err != nil && !errors.Is(err, quotadb.ErrNotFound) {
		return err
	}
	tenant.Status.Reserved = corev1.CanonicalResources(account.Allocated)
	return nil
}

func (o *operator) canListAllTenants(ctx context.Context, u iamv1.UserInfo) bool {
	if o.reg == nil || o.reg.Pool() == nil {
		return true
	}
	allowed, err := access.AllowsGlobal(ctx, o.reg, u, corev1.GroupName, "tenants", iamv1.VerbGet, "*", nil)
	return err == nil && allowed
}

func (o *operator) CanView(ctx context.Context, name string, hint *corev1.Tenant, deletedOwner string) bool {
	u, hasUser := iamv1.UserFrom(ctx)
	if !hasUser || u.Kind == iamv1.SubjectKindSystem {
		return true
	}
	if o.reg != nil && o.reg.Pool() != nil {
		if hint == nil && o.db != nil && name != "" {
			if loaded, err := o.db.Get(ctx, name); err == nil && loaded != nil {
				hint = loaded
			}
		}
		var labels map[string]string
		if hint != nil {
			labels = nonNilTenantLabels(hint.Metadata.Labels)
		}
		targetName := "*"
		if name != "" {
			targetName = name
		}
		if allowed, err := access.AllowsGlobal(ctx, o.reg, u, corev1.GroupName, "tenants", iamv1.VerbGet, targetName, labels); err == nil && allowed {
			return true
		}
	}
	if u.IsAnonymous() || u.Kind == iamv1.SubjectKindNode || name == "" {
		return false
	}
	owner := deletedOwner
	if hint != nil && hint.Spec.Owner != "" {
		owner = hint.Spec.Owner
	}
	if owner != "" && u.Name != "" && owner == u.Name {
		return true
	}
	return o.hasTenantRoleBinding(ctx, name, u)
}

func (o *operator) hasTenantRoleBinding(ctx context.Context, tenantName string, u iamv1.UserInfo) bool {
	if o.trbDB == nil {
		return false
	}
	trbs, err := o.trbDB.List(ctx, db.ListOptions{
		Tenant:        tenantName,
		FieldSelector: map[string]string{"subject": access.SubjectFilter(u)},
	})
	if err != nil || trbs == nil {
		return false
	}
	for _, b := range trbs.Items {
		if b != nil && b.Metadata.Tenant == tenantName && access.SubjectMatches(b.Subjects, u) {
			return true
		}
	}
	return false
}

func (o *operator) Get(ctx context.Context, name string) (*corev1.Tenant, error) {
	tenant, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := o.projectReserved(ctx, tenant); err != nil {
		return nil, err
	}
	return tenant, nil
}

func (o *operator) Create(ctx context.Context, defaultOwner string, tenant *corev1.Tenant) (*corev1.Tenant, error) {
	if err := o.authorizeTenantCreate(ctx, tenant); err != nil {
		return nil, err
	}
	tenant.DefaultMeta()
	if err := tenant.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if _, err := corev1.NormalizeLimits(tenant.Spec.Quota, tenant.Spec.UnlimitedQuota); err != nil {
		return nil, fmt.Errorf("%w: invalid tenant quota: %v", models.ErrBadRequest, err)
	}
	if tenant.Spec.Owner == "" {
		tenant.Spec.Owner = defaultOwner
	}
	ctx, err := o.pinPersonalParent(ctx, tenant)
	if err != nil {
		return nil, err
	}
	tenant.Status.Reserved = corev1.CanonicalResources(corev1.Resources{})
	created, err := o.db.Create(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventCreated, corev1.GroupVersion, "Tenant", "", created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion))
	}
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, tenant *corev1.Tenant) (*corev1.Tenant, error) {
	if _, err := corev1.NormalizeLimits(tenant.Spec.Quota, tenant.Spec.UnlimitedQuota); err != nil {
		return nil, fmt.Errorf("%w: invalid tenant quota: %v", models.ErrBadRequest, err)
	}
	current, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if (tenant.Metadata.UID != "" && tenant.Metadata.UID != current.Metadata.UID) ||
		(tenant.Metadata.ResourceVersion != "" && tenant.Metadata.ResourceVersion != current.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	if current.Metadata.DeletionTimestamp != nil {
		return nil, fmt.Errorf("%w: terminating tenant is immutable", models.ErrConflict)
	}
	if err := preserveOwnerIdentity(current, tenant); err != nil {
		return nil, err
	}
	if err := o.authorizeTenantUpdate(ctx, current, tenant); err != nil {
		return nil, err
	}
	if err := tenant.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	// Lifecycle metadata is server owned even for platform quota administrators.
	tenant.Metadata.DeletionTimestamp = current.Metadata.DeletionTimestamp
	tenant.Metadata.Finalizers = append([]string(nil), current.Metadata.Finalizers...)
	// Bind the write to the same snapshot used for sensitive-field checks even
	// when a caller omitted resourceVersion from a full-object update.
	tenant.Metadata.Name = name
	tenant.Metadata.UID = current.Metadata.UID
	tenant.Metadata.ResourceVersion = current.Metadata.ResourceVersion
	updated, err := o.db.Update(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if err := o.projectReserved(ctx, updated); err != nil {
		return nil, err
	}
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Tenant", "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion))
		if updated.Metadata.Name == builtInAdminName() {
			wake := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, corev1.KindPrewarmJob, "", corev1.PrewarmSyncDefaultsName, 0, "")
			wake.Topic = mq.TopicPrewarmJob
			_ = o.mq.Publish(ctx, mq.TopicPrewarmJob, wake)
		}
	}
	return updated, nil
}

func (o *operator) UpdateStatus(ctx context.Context, name string, tenant *corev1.Tenant) (*corev1.Tenant, error) {
	tenant.Metadata.Name = name
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := o.projectReserved(ctx, existing); err != nil {
		return nil, err
	}
	tenant.Status.Reserved = existing.Status.Reserved
	return o.db.UpdateStatus(ctx, tenant)
}

func (o *operator) pinPersonalParent(ctx context.Context, tenant *corev1.Tenant) (context.Context, error) {
	if tenant.Spec.Type != corev1.TenantTypePersonal {
		if tenant.Metadata.Annotations != nil {
			delete(tenant.Metadata.Annotations, iamv1.AnnotationOwnerUID)
		}
		return ctx, nil
	}
	if tenant.Metadata.Name != tenant.Spec.Owner {
		return nil, fmt.Errorf("%w: personal workspace name must equal its owner", models.ErrBadRequest)
	}
	owner, err := o.userDB.Get(ctx, tenant.Spec.Owner)
	if err != nil {
		return nil, err
	}
	if owner.Metadata.UID == "" || owner.Metadata.DeletionTimestamp != nil {
		return nil, fmt.Errorf("%w: personal workspace owner is terminating", models.ErrConflict)
	}
	if tenant.Metadata.Annotations == nil {
		tenant.Metadata.Annotations = make(map[string]string)
	}
	tenant.Metadata.Annotations[iamv1.AnnotationOwnerUID] = owner.Metadata.UID
	return ctx, nil
}

func preserveOwnerIdentity(current, incoming *corev1.Tenant) error {
	if current.Spec.Type == corev1.TenantTypePersonal && (incoming.Spec.Type != current.Spec.Type || incoming.Spec.Owner != current.Spec.Owner) {
		return fmt.Errorf("%w: personal workspace type and owner are immutable", models.ErrConflict)
	}
	if current.Spec.Type != corev1.TenantTypePersonal && incoming.Spec.Type == corev1.TenantTypePersonal {
		return fmt.Errorf("%w: personal workspace identity must be established at creation", models.ErrConflict)
	}
	if incoming.Metadata.Annotations == nil {
		incoming.Metadata.Annotations = make(map[string]string)
	}
	if uid := current.Metadata.Annotations[iamv1.AnnotationOwnerUID]; uid != "" {
		incoming.Metadata.Annotations[iamv1.AnnotationOwnerUID] = uid
	} else {
		delete(incoming.Metadata.Annotations, iamv1.AnnotationOwnerUID)
	}
	return nil
}

func effectiveTenantLimits(spec corev1.TenantSpec) (corev1.Limits, error) {
	candidate := &corev1.Tenant{Spec: spec}
	candidate.Spec.Quota = maps.Clone(spec.Quota)
	candidate.DefaultMeta()
	return corev1.NormalizeLimits(candidate.Spec.Quota, candidate.Spec.UnlimitedQuota)
}

func (o *operator) authorizeTenantCreate(ctx context.Context, tenant *corev1.Tenant) error {
	user, explicit := iamv1.UserFrom(ctx)
	if !explicit || (user.Kind == iamv1.SubjectKindSystem && !user.IsAnonymous()) {
		return nil
	}
	if user.Kind != iamv1.SubjectKindUser || user.IsAnonymous() {
		return fmt.Errorf("%w: tenant creation requires an authenticated user", models.ErrForbidden)
	}
	if tenant.Spec.Owner == "" {
		tenant.Spec.Owner = user.Name
	}
	if tenant.Spec.Owner != user.Name {
		if err := o.requireGlobalTenantPrivilege(ctx, user, "tenants/owner", tenant.Metadata.Name, tenant.Metadata.Labels); err != nil {
			return err
		}
	}
	if _, err := corev1.NormalizeLimits(tenant.Spec.Quota, tenant.Spec.UnlimitedQuota); err != nil {
		return fmt.Errorf("%w: invalid tenant quota: %v", models.ErrBadRequest, err)
	}
	isAdmin, err := o.hasGlobalTenantPrivilege(ctx, user, corev1.ResourceTenantQuota, tenant.Metadata.Name, tenant.Metadata.Labels)
	if err != nil {
		return err
	}
	if !isAdmin {
		return authorizeNonAdminTenantCreate(tenant)
	}
	return nil
}

func authorizeNonAdminTenantCreate(tenant *corev1.Tenant) error {
	if tenant.Spec.Type == corev1.TenantTypePersonal {
		return fmt.Errorf("%w: personal workspaces are provisioned automatically", models.ErrForbidden)
	}
	limits, err := effectiveTenantLimits(tenant.Spec)
	if err != nil {
		return fmt.Errorf("%w: invalid tenant quota: %v", models.ErrBadRequest, err)
	}
	defLimits, err := effectiveTenantLimits(corev1.TenantSpec{})
	if err != nil {
		return err
	}
	if limits != defLimits {
		return fmt.Errorf("%w: changing tenant quota requires global tenants/quota update permission", models.ErrForbidden)
	}
	return nil
}

func (o *operator) authorizeTenantUpdate(ctx context.Context, current, tenant *corev1.Tenant) error {
	before, err := effectiveTenantLimits(current.Spec)
	if err != nil {
		return err
	}
	after, err := effectiveTenantLimits(tenant.Spec)
	if err != nil {
		return fmt.Errorf("%w: invalid tenant quota: %v", models.ErrBadRequest, err)
	}
	return o.authorizeTenantUpdateIdentityAndQuota(ctx, current, tenant, before != after)
}

func (o *operator) authorizeTenantUpdateIdentityAndQuota(ctx context.Context, current, tenant *corev1.Tenant, quotaChanged bool) error {
	user, explicit := iamv1.UserFrom(ctx)
	if !explicit || (user.Kind == iamv1.SubjectKindSystem && !user.IsAnonymous()) {
		return nil
	}
	if user.Kind != iamv1.SubjectKindUser || user.IsAnonymous() {
		return fmt.Errorf("%w: tenant updates require an authenticated user", models.ErrForbidden)
	}
	if tenant.Spec.Owner != current.Spec.Owner {
		if err := o.requireGlobalTenantPrivilege(ctx, user, "tenants/owner", current.Metadata.Name, current.Metadata.Labels); err != nil {
			return err
		}
	}
	if quotaChanged {
		return o.requireGlobalTenantPrivilege(ctx, user, corev1.ResourceTenantQuota, current.Metadata.Name, current.Metadata.Labels)
	}
	return nil
}

func (o *operator) hasGlobalTenantPrivilege(ctx context.Context, user iamv1.UserInfo, resource, name string, labels map[string]string) (bool, error) {
	return access.AllowsGlobal(ctx, o.reg, user, corev1.GroupName, resource, iamv1.VerbUpdate, name, labels)
}

func (o *operator) requireGlobalTenantPrivilege(ctx context.Context, user iamv1.UserInfo, resource, name string, labels map[string]string) error {
	allowed, err := access.AllowsGlobal(ctx, o.reg, user, corev1.GroupName, resource, iamv1.VerbUpdate, name, labels)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%w: changing tenant %s requires global %s update permission", models.ErrForbidden, name, resource)
	}
	return nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	if name == builtInAdminName() {
		return fmt.Errorf("%w: cannot delete built-in administrator tenant %q", models.ErrConflict, name)
	}
	initial, err := o.db.Get(ctx, name)
	if err != nil {
		return err
	}
	if initial.Spec.Type == corev1.TenantTypePersonal && initial.Spec.Owner != "" && o.userDB != nil {
		if _, err := o.userDB.Get(ctx, initial.Spec.Owner); err == nil {
			return fmt.Errorf("%w: delete the owner before deleting a personal workspace", models.ErrConflict)
		} else if !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	return RequestDeletion(ctx, o.reg, o.mq, name, initial.Metadata.UID)
}

func RequestDeletion(ctx context.Context, reg *repo.Registry, bus mq.Interface, name, expectedUID string) error {
	if expectedUID == "" {
		return db.ErrConflict
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	current, err := tenantDB.Get(ctx, name)
	if err != nil {
		return err
	}
	if current.Metadata.UID != expectedUID {
		return db.ErrConflict
	}
	if current.Metadata.DeletionTimestamp == nil {
		now := time.Now().UTC()
		current.Metadata.DeletionTimestamp = &now
		current.Metadata.Finalizers = append(current.Metadata.Finalizers, corev1.FinalizerTenantCleanup)
		current, err = tenantDB.UpdateObject(ctx, current)
		if err != nil {
			return err
		}
		current.Status.Phase = corev1.TenantPhaseTerminating
		if updatedStatus, sErr := tenantDB.UpdateStatus(ctx, current); sErr == nil {
			current = updatedStatus
		}
	}
	if bus != nil {
		_ = bus.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Tenant", "", name, current.Metadata.Generation, current.Metadata.ResourceVersion))
	}
	return nil
}
