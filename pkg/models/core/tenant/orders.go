package tenant

import (
	"context"
	"fmt"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/subscription"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
)

func actorNameFrom(ctx context.Context) string {
	if u, ok := iamv1.UserFrom(ctx); ok && u.Name != "" {
		return u.Name
	}
	return iamv1.PrincipalSystem
}

func (o *operator) Redeem(ctx context.Context, tenantName string, req corev1.RedeemRequest) (*corev1.RedeemResponse, error) {
	return subscription.NewService(o.reg, o.mq).Redeem(ctx, actorNameFrom(ctx), tenantName, req.Code)
}

func (o *operator) ListOrders(ctx context.Context, tenantName string, opts db.ListOptions) (*metav1.List[corev1.QuotaOrder], error) {
	return subscription.NewService(o.reg, o.mq).ListOrders(ctx, tenantName, opts)
}

func (o *operator) CreateOrder(ctx context.Context, tenantName string, req corev1.CreateQuotaOrderRequest) (*corev1.QuotaOrder, error) {
	if err := o.requireGlobalQuotaAdmin(ctx, tenantName); err != nil {
		return nil, err
	}
	return subscription.NewService(o.reg, o.mq).CreateAdminOrder(ctx, actorNameFrom(ctx), tenantName, req)
}

func (o *operator) CancelOrder(ctx context.Context, tenantName, orderID string, req corev1.PatchQuotaOrderRequest) (*corev1.QuotaOrder, error) {
	if req.State != corev1.QuotaOrderStateCancelled {
		return nil, fmt.Errorf("%w: only state=Cancelled is supported", models.ErrBadRequest)
	}
	if err := o.requireGlobalQuotaAdmin(ctx, tenantName); err != nil {
		return nil, err
	}
	return subscription.NewService(o.reg, o.mq).CancelOrder(ctx, actorNameFrom(ctx), tenantName, orderID, req.Reason)
}

func (o *operator) requireGlobalQuotaAdmin(ctx context.Context, tenantName string) error {
	user, explicit := iamv1.UserFrom(ctx)
	if !explicit || (user.Kind == iamv1.SubjectKindSystem && !user.IsAnonymous()) {
		return nil
	}
	if user.Kind != iamv1.SubjectKindUser || user.IsAnonymous() {
		return fmt.Errorf("%w: managing tenant orders requires global tenants/quota permission", models.ErrForbidden)
	}
	var labels map[string]string
	if t, err := o.db.Get(ctx, tenantName); err == nil && t != nil {
		labels = t.Metadata.Labels
	}
	return o.requireGlobalTenantPrivilege(ctx, user, corev1.ResourceTenantQuota, tenantName, labels)
}

func (o *operator) ReclaimExpired(ctx context.Context, tenantName string) (*corev1.ReclaimTenantResponse, error) {
	if err := o.requireGlobalQuotaAdmin(ctx, tenantName); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tenant, err := o.db.Get(ctx, tenantName)
	if err != nil {
		return nil, err
	}
	if err := ensureTenantReclaimable(tenant, now); err != nil {
		o.publishTenantReconcile(ctx, tenant)
		return nil, err
	}
	reclaimed, err := o.markTenantInstancesDeleted(ctx, tenantName)
	if err != nil {
		return nil, err
	}
	return &corev1.ReclaimTenantResponse{
		TypeMeta:           metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindReclaimTenantResponse},
		Metadata:           metav1.ObjectMeta{Name: tenantName, Tenant: tenantName, UID: tenant.Metadata.UID},
		Tenant:             tenantName,
		ReclaimedCount:     len(reclaimed),
		ReclaimedInstances: reclaimed,
	}, nil
}

func (o *operator) publishTenantReconcile(ctx context.Context, tenant *corev1.Tenant) {
	if o.mq == nil || tenant == nil {
		return
	}
	_ = o.mq.Publish(ctx, mq.TopicTenant, mq.NewEvent(
		mq.EventReconcile, corev1.GroupVersion, "Tenant", "",
		tenant.Metadata.Name, tenant.Metadata.Generation, tenant.Metadata.ResourceVersion,
	))
}

func (o *operator) markTenantInstancesDeleted(ctx context.Context, tenantName string) ([]string, error) {
	instDB := instancedb.NewDB(o.reg.Pool())
	list, err := instDB.List(ctx, db.ListOptions{Tenant: tenantName})
	if err != nil {
		return nil, err
	}
	reclaimed := make([]string, 0)
	for _, inst := range list.Items {
		if inst == nil || inst.Metadata.DeletionTimestamp != nil {
			continue
		}
		updated, delErr := instancemodel.RequestDelete(ctx, instDB, tenantName, inst.Metadata.Name, inst.Metadata.UID)
		if delErr != nil {
			return nil, delErr
		}
		reclaimed = append(reclaimed, inst.Metadata.Name)
		if o.mq != nil && updated != nil {
			_ = o.mq.Publish(ctx, mq.TopicInstance, mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Instance", tenantName, updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion))
		}
	}
	return reclaimed, nil
}

func ensureTenantReclaimable(tenant *corev1.Tenant, now time.Time) error {
	if tenant == nil || tenant.Spec.BillingMode != corev1.TenantBillingModeSubscription {
		return fmt.Errorf("%w: only Subscription tenants can be reclaimed after grace period", models.ErrConflict)
	}
	sub := tenant.Status.Subscription
	if sub == nil || sub.State != corev1.SubscriptionStateSuspended || sub.GraceUntil == nil || now.Before(*sub.GraceUntil) {
		return fmt.Errorf("%w: tenant has not passed subscription grace period", models.ErrConflict)
	}
	return nil
}
