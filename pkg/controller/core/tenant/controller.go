package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"sync"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	tenantrolemodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrole"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	tenantroledb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/tenantrole"
	tenantrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/tenantrolebinding"
)

const tenantResyncBatchSize = 200

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct {
	mu           sync.Mutex
	resyncOffset int
}

func (c *Controller) Name() string  { return "01-tenant-and-quota" }
func (c *Controller) Topic() string { return mq.TopicTenant }

// Resync inspects a bounded page of Tenants per pass
// and queries instances scoped per tenant rather than scanning all cluster instances at once.
func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, _ *ctrlruntime.Config) ([]mq.Event, error) {
	c.mu.Lock()
	offset := c.resyncOffset
	c.mu.Unlock()

	tenants, err := tenantdb.NewDB(reg.Pool()).List(ctx, db.ListOptions{
		Limit:          tenantResyncBatchSize,
		Offset:         offset,
		SkipTotalCount: true,
	})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(tenants.Items) == tenantResyncBatchSize {
		c.resyncOffset = offset + tenantResyncBatchSize
	} else {
		c.resyncOffset = 0
	}
	c.mu.Unlock()

	var events []mq.Event
	for _, t := range tenants.Items {
		if t == nil {
			continue
		}
		needsSync, err := shouldResyncTenant(ctx, reg, t)
		if err != nil {
			return nil, err
		}
		if needsSync {
			ev := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", t.Metadata.Name, t.Metadata.Generation, t.Metadata.ResourceVersion)
			ev.Topic = mq.TopicTenant
			events = append(events, ev)
		}
	}
	return events, nil
}

var allQuotaDimensions = []corev1.ResourceName{
	corev1.ResourceCPU,
	corev1.ResourceMemory,
	corev1.ResourceStorage,
	corev1.ResourceInstances,
	corev1.ResourceBackups,
	corev1.ResourceBackupStorage,
}

func hasAllUnlimitedQuota(t *corev1.Tenant) bool {
	if t == nil || t.Spec.BillingMode != corev1.TenantBillingModeManual {
		return false
	}
	for _, dim := range allQuotaDimensions {
		if !t.Spec.IsQuotaUnlimited(dim) {
			return false
		}
	}
	return true
}

func shouldResyncTenant(ctx context.Context, reg *repo.Registry, t *corev1.Tenant) (bool, error) {
	if t.Metadata.DeletionTimestamp != nil || !hasAllUnlimitedQuota(t) {
		return true, nil
	}
	instList, err := instancedb.NewDB(reg.Pool()).List(ctx, db.ListOptions{Tenant: t.Metadata.Name})
	if err != nil {
		return false, err
	}
	account, accountErr := quotadb.NewDB(reg.Pool()).GetAccount(ctx, t.Metadata.UID)
	version, versionErr := strconv.ParseInt(t.Metadata.ResourceVersion, 10, 64)
	if accountErr != nil || versionErr != nil || account.PolicyVersion == 0 || account.UpstreamVersion < version {
		return true, nil
	}
	expectedUsage := computeUsage(instList.Items)
	return t.Status.Subscription != nil || needsReconcile(t, expectedUsage), nil
}

// Reconcile is the unified, idempotent reconciliation handler shared by both the MQ consumer
// on topic "core.v1.tenants" and the periodic Resync loop.
func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, _ *ctrlruntime.Config, event mq.Event) error {
	if event.Type == mq.EventDeleted {
		return nil
	}
	name := event.Name
	if name == "" {
		name = event.Tenant
	}
	if name == "" {
		return nil
	}
	t, err := tenantdb.NewDB(reg.Pool()).Get(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if t.Metadata.DeletionTimestamp != nil {
		return reconcileTenantDeletion(ctx, reg, bus, t)
	}
	if err := tenantrolemodel.EnsureBuiltInRoles(ctx, reg, t.Metadata.Name, t.Spec.Owner); err != nil {
		return err
	}
	return reconcileActiveTenantStatus(ctx, reg, bus, name, t.Metadata.UID)
}

func reconcileActiveTenantStatus(ctx context.Context, reg *repo.Registry, bus mq.Interface, name, expectedUID string) error {
	tenantDB := tenantdb.NewDB(reg.Pool())
	cur, err := tenantDB.Get(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if cur.Metadata.UID != expectedUID {
		return db.ErrConflict
	}
	if cur.Metadata.DeletionTimestamp != nil {
		return reconcileTenantDeletion(ctx, reg, bus, cur)
	}
	instances, err := instancedb.NewDB(reg.Pool()).List(ctx, db.ListOptions{Tenant: cur.Metadata.Name})
	if err != nil {
		return err
	}
	expectedUsage := computeUsage(instances.Items)
	if !hasAllUnlimitedQuota(cur) {
		cur.Spec.BillingMode = corev1.TenantBillingModeManual
		cur.Spec.Quota = corev1.ResourceList{}
		cur.Spec.UnlimitedQuota = append([]corev1.ResourceName(nil), allQuotaDimensions...)
		updatedSpec, uErr := tenantDB.Update(ctx, cur)
		if uErr != nil {
			return uErr
		}
		cur = updatedSpec
	}
	return reconcileManualStatus(ctx, reg, bus, tenantDB, cur, expectedUsage, instances.Items)
}

func reconcileManualStatus(ctx context.Context, reg *repo.Registry, bus mq.Interface, tenantDB *tenantdb.DB, cur *corev1.Tenant, expectedUsage corev1.ResourceList, instances []*corev1.Instance) error {
	if err := syncTenantAccount(ctx, reg, cur); err != nil {
		return err
	}
	prevPhase := cur.Status.Phase
	updated := cur
	if cur.Status.Subscription != nil || needsReconcile(cur, expectedUsage) {
		cur.Status.ObservedGeneration = cur.Metadata.Generation
		cur.Status.Phase = corev1.TenantPhaseActive
		cur.Status.Subscription = nil
		cur.Status.Usage = expectedUsage
		metav1.SetCondition(&cur.Status.Conditions, metav1.Condition{
			Type: metav1.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: cur.Metadata.Generation,
			Reason: corev1.ReasonTenantActive, Message: "Tenant quota and roles reconciled",
		})
		var err error
		updated, err = tenantDB.UpdateStatus(ctx, cur)
		if err != nil {
			return err
		}
		if err := syncTenantAccount(ctx, reg, updated); err != nil {
			return err
		}
	}
	if prevPhase != updated.Status.Phase {
		wakeTenantInstances(ctx, bus, instances)
	}
	return nil
}

func wakeTenantInstances(ctx context.Context, bus mq.Interface, instances []*corev1.Instance) {
	if bus == nil {
		return
	}
	for _, inst := range instances {
		if inst == nil || inst.Metadata.DeletionTimestamp != nil {
			continue
		}
		ev := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Instance", inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.Generation, inst.Metadata.ResourceVersion)
		ev.Topic = mq.TopicInstance
		_ = bus.Publish(ctx, mq.TopicInstance, ev)
	}
}

func syncTenantAccount(ctx context.Context, reg *repo.Registry, tenant *corev1.Tenant) error {
	version, err := strconv.ParseInt(tenant.Metadata.ResourceVersion, 10, 64)
	if err != nil || version <= 0 || tenant.Metadata.UID == "" {
		return fmt.Errorf("tenant %q requires a UID and positive resourceVersion for quota policy", tenant.Metadata.Name)
	}
	limits, err := corev1.NormalizeLimits(tenant.Spec.Quota, tenant.Spec.UnlimitedQuota)
	if err != nil {
		return err
	}
	_, _, err = quotadb.NewDB(reg.Pool()).SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       tenant.Metadata.UID,
		UpstreamVersion: version,
		Target:          quotadb.QuotaPolicy{Limits: limits},
		Kind:            corev1.QuotaPolicyChangeKindAdminAdjust,
	})
	return err
}

func computeUsage(instances []*corev1.Instance) corev1.ResourceList {
	var used corev1.Resources
	for _, inst := range instances {
		if inst == nil || inst.Metadata.DeletionTimestamp != nil {
			continue
		}
		used.InstanceCount++
		if inst.Status.Usage != nil {
			used.CPUMillicores += inst.Status.Usage.CPUMillicores
			used.MemoryBytes += inst.Status.Usage.MemoryBytes
			used.StorageBytes += inst.Status.Usage.StorageBytes
		}
		for _, backup := range inst.Status.Backups {
			used.BackupsCount++
			used.BackupStorageBytes += backup.SizeBytes
		}
	}
	return corev1.CanonicalResources(used)
}

func needsReconcile(t *corev1.Tenant, expectedUsage corev1.ResourceList) bool {
	if t == nil {
		return false
	}
	if t.Status.Phase != corev1.TenantPhaseActive || t.Status.ObservedGeneration < t.Metadata.Generation {
		return true
	}
	return !maps.Equal(t.Status.Usage, expectedUsage)
}

func reconcileTenantDeletion(ctx context.Context, reg *repo.Registry, bus mq.Interface, tenant *corev1.Tenant) error {
	empty, err := drainTenantInstancesAndQuota(ctx, reg, bus, tenant)
	if err != nil || !empty {
		return err
	}
	rbacDrained, err := drainTenantRBACResources(ctx, reg, bus, tenant.Metadata.Name)
	if err != nil || !rbacDrained {
		return err
	}
	return finalizeTenantDeletion(ctx, reg, bus, tenant)
}

func drainTenantInstancesAndQuota(ctx context.Context, reg *repo.Registry, bus mq.Interface, tenant *corev1.Tenant) (bool, error) {
	instDB := instancedb.NewDB(reg.Pool())
	children, err := instDB.List(ctx, db.ListOptions{Tenant: tenant.Metadata.Name, Limit: tenantResyncBatchSize, SkipTotalCount: true})
	if err != nil {
		return false, err
	}
	for _, child := range children.Items {
		if child == nil {
			continue
		}
		if child.Status.ResourceControl == nil || child.Status.ResourceControl.TenantUID != tenant.Metadata.UID {
			return false, db.ErrConflict
		}
		marked, err := instancemodel.RequestDelete(ctx, instDB, child.Metadata.Tenant, child.Metadata.Name, child.Metadata.UID)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return false, err
		}
		if marked != nil && bus != nil {
			_ = bus.Publish(ctx, mq.TopicInstance, mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Instance", marked.Metadata.Tenant, marked.Metadata.Name, marked.Metadata.Generation, marked.Metadata.ResourceVersion))
		}
	}
	if len(children.Items) != 0 {
		return false, nil
	}
	account, err := quotadb.NewDB(reg.Pool()).GetAccount(ctx, tenant.Metadata.UID)
	if err != nil && !errors.Is(err, quotadb.ErrNotFound) {
		return false, err
	}
	if err == nil && !account.Allocated.IsZero() {
		return false, nil
	}
	return true, nil
}

func drainTenantRBACResources(ctx context.Context, reg *repo.Registry, bus mq.Interface, tenantName string) (bool, error) {
	trbDB := tenantrolebindingdb.NewDB(reg.Pool())
	trDB := tenantroledb.NewDB(reg.Pool())
	bindings, err := trbDB.List(ctx, db.ListOptions{Tenant: tenantName, Limit: tenantResyncBatchSize, SkipTotalCount: true})
	if err != nil {
		return false, err
	}
	for _, binding := range bindings.Items {
		if err := trbDB.DeleteIfUID(ctx, binding.Metadata.Tenant, binding.Metadata.Name, binding.Metadata.UID); err != nil && !errors.Is(err, db.ErrNotFound) {
			return false, err
		}
		if bus != nil {
			_ = bus.Publish(ctx, mq.TopicTenantRoleBinding, mq.NewEvent(mq.EventDeleted, iamv1.GroupVersion, "TenantRoleBinding", binding.Metadata.Tenant, binding.Metadata.Name, 0, ""))
		}
	}
	if len(bindings.Items) == tenantResyncBatchSize {
		return false, nil
	}
	roles, err := trDB.List(ctx, db.ListOptions{Tenant: tenantName, Limit: tenantResyncBatchSize, SkipTotalCount: true})
	if err != nil {
		return false, err
	}
	for _, role := range roles.Items {
		if err := trDB.DeleteIfUID(ctx, role.Metadata.Tenant, role.Metadata.Name, role.Metadata.UID); err != nil && !errors.Is(err, db.ErrNotFound) {
			return false, err
		}
		if bus != nil {
			_ = bus.Publish(ctx, mq.TopicTenantRole, mq.NewEvent(mq.EventDeleted, iamv1.GroupVersion, "TenantRole", role.Metadata.Tenant, role.Metadata.Name, 0, ""))
		}
	}
	return len(roles.Items) < tenantResyncBatchSize, nil
}

func finalizeTenantDeletion(ctx context.Context, reg *repo.Registry, bus mq.Interface, tenant *corev1.Tenant) error {
	tenantDB := tenantdb.NewDB(reg.Pool())
	current, err := tenantDB.Get(ctx, tenant.Metadata.Name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if current.Metadata.UID != tenant.Metadata.UID || current.Metadata.DeletionTimestamp == nil {
		return db.ErrConflict
	}
	finalizers := make([]string, 0, len(current.Metadata.Finalizers))
	for _, finalizer := range current.Metadata.Finalizers {
		if finalizer != corev1.FinalizerTenantCleanup {
			finalizers = append(finalizers, finalizer)
		}
	}
	if len(finalizers) != len(current.Metadata.Finalizers) {
		current.Metadata.Finalizers = finalizers
		current, err = tenantDB.UpdateObject(ctx, current)
		if err != nil {
			return err
		}
	}
	if len(current.Metadata.Finalizers) > 0 {
		return nil
	}
	if err := tenantDB.DeleteIfUID(ctx, current.Metadata.Name, current.Metadata.UID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if bus != nil {
		ev := mq.NewEvent(mq.EventDeleted, corev1.GroupVersion, "Tenant", "", current.Metadata.Name, current.Metadata.Generation, current.Metadata.ResourceVersion)
		attrs := map[string]string{
			"uid":   current.Metadata.UID,
			"owner": current.Spec.Owner,
		}
		labels := current.Metadata.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		if raw, err := json.Marshal(labels); err == nil {
			attrs["labels"] = string(raw)
		}
		ev.Attributes = attrs
		_ = bus.Publish(ctx, mq.TopicTenant, ev)
	}
	return nil
}
