package subscription

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	plandb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/resourceplan"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
)

// ReconcileSubscriptionPolicy converges Spec.Quota, quota_accounts, and quota_orders for a Subscription tenant.
func ReconcileSubscriptionPolicy(ctx context.Context, reg *repo.Registry, tenantName string, now time.Time) (*corev1.Tenant, FoldResult, error) {
	if reg == nil || reg.Pool() == nil {
		return nil, FoldResult{}, errors.New("db registry is nil")
	}
	pool := reg.Pool()
	tenantDB := tenantdb.NewDB(pool)
	quotaDB := quotadb.NewDB(pool)
	planDB := plandb.NewDB(pool)
	tenant, err := tenantDB.Get(ctx, tenantName)
	if err != nil {
		return nil, FoldResult{}, err
	}
	if tenant.Metadata.DeletionTimestamp != nil || tenant.Spec.BillingMode != corev1.TenantBillingModeSubscription {
		return tenant, FoldResult{}, nil
	}
	if err := ensurePersonalTrialOrder(ctx, reg, tenant, quotaDB, planDB, now); err != nil {
		return nil, FoldResult{}, err
	}
	res, err := computeFoldForTenant(ctx, quotaDB, planDB, tenant.Metadata.UID, now)
	if err != nil {
		return nil, FoldResult{}, err
	}
	tenant, err = syncTenantPolicyObject(ctx, tenantDB, quotaDB, tenant.Metadata.Name, res)
	if err != nil {
		return nil, FoldResult{}, err
	}
	if err := syncSubscriptionQuotaLedger(ctx, quotaDB, tenant, res, now); err != nil {
		return nil, FoldResult{}, err
	}
	return tenant, res, nil
}

// ReconcileSubscriptionTenant converges policy and persists Subscription + Phase status onto tenant_states.
func ReconcileSubscriptionTenant(ctx context.Context, reg *repo.Registry, tenantName string, now time.Time) (*corev1.Tenant, error) {
	tenant, res, err := ReconcileSubscriptionPolicy(ctx, reg, tenantName, now)
	if err != nil || tenant == nil || tenant.Spec.BillingMode != corev1.TenantBillingModeSubscription {
		return tenant, err
	}
	sub := res.Subscription
	tenant.Status.ObservedGeneration = tenant.Metadata.Generation
	tenant.Status.Phase = res.Phase
	tenant.Status.Subscription = &sub
	setSubscriptionCondition(tenant)
	return tenantdb.NewDB(reg.Pool()).UpdateStatus(ctx, tenant)
}

func computeFoldForTenant(ctx context.Context, quotaDB *quotadb.DB, planDB *plandb.DB, tenantUID string, now time.Time) (FoldResult, error) {
	orders, err := quotaDB.ListFoldableOrders(ctx, tenantUID)
	if err != nil {
		return FoldResult{}, err
	}
	plansMap, err := loadPlansMap(ctx, planDB)
	if err != nil {
		return FoldResult{}, err
	}
	return Fold(orders, plansMap, now)
}

func loadPlansMap(ctx context.Context, planDB *plandb.DB) (map[string]*corev1.ResourcePlan, error) {
	list, err := planDB.List(ctx, db.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make(map[string]*corev1.ResourcePlan, len(list.Items))
	for _, p := range list.Items {
		if p != nil {
			out[p.Metadata.Name] = p
		}
	}
	return out, nil
}

func ensurePersonalTrialOrder(ctx context.Context, reg *repo.Registry, tenant *corev1.Tenant, quotaDB *quotadb.DB, planDB *plandb.DB, now time.Time) error {
	if tenant.Spec.Type != corev1.TenantTypePersonal {
		return nil
	}
	hasAny, err := quotaDB.HasAnyOrders(ctx, tenant.Metadata.UID)
	if err != nil || hasAny {
		return err
	}
	userUID := tenant.Metadata.Annotations[iamv1.AnnotationOwnerUID]
	if userUID == "" && tenant.Spec.Owner != "" {
		if u, uErr := userdb.NewDB(reg.Pool()).Get(ctx, tenant.Spec.Owner); uErr == nil && u != nil {
			userUID = u.Metadata.UID
		}
	}
	if userUID == "" {
		return nil
	}
	plan, err := planDB.Get(ctx, corev1.BuiltInTrialPlanName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	t0 := tenant.Metadata.CreationTimestamp.UTC().Truncate(time.Microsecond)
	if t0.IsZero() {
		t0 = now.UTC().Truncate(time.Microsecond)
	}
	exp := t0.Add(corev1.DefaultSubscriptionTrialDays * 24 * time.Hour)
	_, err = quotaDB.InsertOrder(ctx, corev1.QuotaOrder{
		ID: "trial:" + userUID, TenantUID: tenant.Metadata.UID,
		OrderKind: corev1.QuotaOrderKindBasePlan, PlanName: plan.Metadata.Name,
		Source: corev1.QuotaOrderSourceTrial, SourceRef: userUID,
		Reason: "Personal workspace 7-day trial", QuotaDelta: plan.Quota,
		DurationDays: corev1.DefaultSubscriptionTrialDays, State: corev1.QuotaOrderStatePaid,
		PaidAt: &t0, ExpiresAt: &exp, CreatedBy: tenant.Spec.Owner,
	})
	return err
}

func syncTenantPolicyObject(ctx context.Context, tenantDB *tenantdb.DB, quotaDB *quotadb.DB, name string, res FoldResult) (*corev1.Tenant, error) {
	targetPolicy := quotadb.QuotaPolicy{Limits: res.EffectiveLimits, ExpiresAt: res.PolicyExpiresAt}.Normalize()
	var wantExp string
	if targetPolicy.ExpiresAt != nil {
		wantExp = targetPolicy.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	for range 6 {
		cur, err := tenantDB.Get(ctx, name)
		if err != nil {
			return nil, err
		}
		acct, acctErr := quotaDB.GetAccount(ctx, cur.Metadata.UID)
		curRV, _ := strconv.ParseInt(cur.Metadata.ResourceVersion, 10, 64)
		specChanged := !maps.Equal(cur.Spec.Quota, res.EffectiveQuota) || len(cur.Spec.UnlimitedQuota) > 0
		expChanged := cur.Metadata.Annotations[corev1.AnnotationSubscriptionExpiresAt] != wantExp
		acctOutOfSyncAtRV := acctErr == nil && acct.PolicyVersion > 0 && curRV <= acct.UpstreamVersion && !acct.Policy().Equal(targetPolicy)
		if !specChanged && !expChanged && !acctOutOfSyncAtRV {
			return cur, nil
		}
		cur.Spec.Quota = maps.Clone(res.EffectiveQuota)
		cur.Spec.UnlimitedQuota = nil
		setSubscriptionExpiresAnnotation(cur, wantExp)
		updated, err := tenantDB.UpdateObject(ctx, cur)
		if errors.Is(err, db.ErrConflict) {
			continue
		}
		return updated, err
	}
	return nil, db.ErrConflict
}

func setSubscriptionExpiresAnnotation(cur *corev1.Tenant, wantExp string) {
	ann := maps.Clone(cur.Metadata.Annotations)
	if wantExp == "" {
		delete(ann, corev1.AnnotationSubscriptionExpiresAt)
		if len(ann) == 0 {
			ann = nil
		}
	} else {
		if ann == nil {
			ann = map[string]string{}
		}
		ann[corev1.AnnotationSubscriptionExpiresAt] = wantExp
	}
	cur.Metadata.Annotations = ann
}

func syncSubscriptionQuotaLedger(ctx context.Context, quotaDB *quotadb.DB, tenant *corev1.Tenant, res FoldResult, now time.Time) error {
	version, err := strconv.ParseInt(tenant.Metadata.ResourceVersion, 10, 64)
	if err != nil || version <= 0 {
		version = 1
	}
	acct, err := quotaDB.GetAccount(ctx, tenant.Metadata.UID)
	if err != nil && !errors.Is(err, quotadb.ErrNotFound) {
		return err
	}
	kind := corev1.QuotaPolicyChangeKindSubscription
	if acct.PolicyVersion == 0 {
		kind = corev1.QuotaPolicyChangeKindBootstrap
	} else if res.Subscription.State == corev1.SubscriptionStateSuspended {
		kind = corev1.QuotaPolicyChangeKindExpiry
	}
	_, chg, err := quotaDB.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       tenant.Metadata.UID,
		UpstreamVersion: version,
		Kind:            kind,
		OrderIDs:        res.OrderIDs,
		Target:          quotadb.QuotaPolicy{Limits: res.EffectiveLimits, ExpiresAt: res.PolicyExpiresAt},
	})
	if err != nil {
		return err
	}
	if chg != nil && len(res.OrderIDs) > 0 {
		if err := quotaDB.MarkOrdersApplied(ctx, tenant.Metadata.UID, res.OrderIDs, chg.Plan.ToVersion, now); err != nil {
			return err
		}
	}
	return quotaDB.ReconcileAppliedOrders(ctx, tenant.Metadata.UID)
}

func setSubscriptionCondition(tenant *corev1.Tenant) {
	status, reason, msg := metav1.ConditionTrue, corev1.ReasonTenantActive, "Tenant subscription and quota reconciled"
	if tenant.Status.Phase == corev1.TenantPhaseSuspended {
		status, reason, msg = metav1.ConditionFalse, corev1.ReasonTenantSuspended, "Tenant subscription expired past grace period"
	}
	metav1.SetCondition(&tenant.Status.Conditions, metav1.Condition{
		Type: metav1.ConditionTypeReady, Status: status,
		ObservedGeneration: tenant.Metadata.Generation, Reason: reason, Message: msg,
	})
}

func publishTenantReconcile(ctx context.Context, bus mq.Interface, tenant *corev1.Tenant) {
	if bus == nil || tenant == nil {
		return
	}
	ev := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", tenant.Metadata.Name, tenant.Metadata.Generation, tenant.Metadata.ResourceVersion)
	ev.Topic = mq.TopicTenant
	_ = bus.Publish(ctx, mq.TopicTenant, ev)
}
