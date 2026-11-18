package tenant

import (
	"context"
	"fmt"
	"strconv"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func (o *operator) ListQuotaChanges(ctx context.Context, tenantName string, opts db.ListOptions) (*metav1.List[*corev1.QuotaPolicyChange], error) {
	tenant, err := o.db.Get(ctx, tenantName)
	if err != nil {
		return nil, err
	}
	if !o.CanView(ctx, tenantName, tenant, "") {
		return nil, fmt.Errorf("%w: access denied to tenant %s", models.ErrForbidden, tenantName)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	changes, total, policyVersion, err := o.quotaDB.ListPolicyChanges(ctx, tenant.Metadata.UID, limit, offset)
	if err != nil {
		return nil, err
	}
	items := make([]*corev1.QuotaPolicyChange, 0, len(changes))
	for _, chg := range changes {
		items = append(items, toQuotaPolicyChange(tenant.Metadata.Name, chg))
	}
	return &metav1.List[*corev1.QuotaPolicyChange]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindQuotaPolicyChangeList},
		Metadata: metav1.ListMeta{
			ResourceVersion: strconv.FormatInt(policyVersion, 10),
			Continue:        pgutil.NextContinue(offset, len(items), total),
			TotalCount:      total,
		},
		Items: items,
	}, nil
}

func (o *operator) VerifyQuotaChanges(ctx context.Context, tenantName string) (*corev1.QuotaPolicyVerification, error) {
	u, hasUser := iamv1.UserFrom(ctx)
	if hasUser && u.Kind != iamv1.SubjectKindSystem {
		if u.IsAnonymous() || !o.canListAllTenants(ctx, u) {
			return nil, fmt.Errorf("%w: quota ledger verification requires platform administrator access", models.ErrForbidden)
		}
	}
	tenant, err := o.db.Get(ctx, tenantName)
	if err != nil {
		return nil, err
	}
	report, err := o.quotaDB.VerifyPolicyChain(ctx, tenant.Metadata.UID)
	if err != nil {
		return nil, err
	}
	report.TypeMeta = metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindQuotaPolicyVerification}
	report.Metadata = metav1.ObjectMeta{
		Name:              tenant.Metadata.Name,
		Tenant:            tenant.Metadata.Name,
		UID:               tenant.Metadata.UID,
		CreationTimestamp: tenant.Metadata.CreationTimestamp,
	}
	return &report, nil
}

func toQuotaPolicySnapshot(p quotadb.QuotaPolicy) corev1.QuotaPolicySnapshot {
	quotas, unlimited := corev1.FormatLimits(p.Limits)
	return corev1.QuotaPolicySnapshot{
		Quota:          quotas,
		UnlimitedQuota: unlimited,
		ExpiresAt:      p.ExpiresAt,
	}
}

func toQuotaPolicyChange(tenantName string, chg quotadb.PolicyChange) *corev1.QuotaPolicyChange {
	oldSnap := corev1.QuotaPolicySnapshot{}
	if chg.Plan.FromVersion > 0 {
		oldSnap = toQuotaPolicySnapshot(chg.Plan.Old)
	}
	return &corev1.QuotaPolicyChange{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindQuotaPolicyChange},
		Metadata: metav1.ObjectMeta{
			Name:              fmt.Sprintf("v%d", chg.Plan.ToVersion),
			Tenant:            tenantName,
			UID:               chg.Plan.ChangeID,
			ResourceVersion:   strconv.FormatInt(chg.Plan.ToVersion, 10),
			CreationTimestamp: chg.CreatedAt,
		},
		ChangeKind:            chg.Plan.Kind,
		FromVersion:           chg.Plan.FromVersion,
		ToVersion:             chg.Plan.ToVersion,
		TenantResourceVersion: chg.Plan.TenantResourceVersion,
		OrderIDs:              chg.Plan.OrderIDs,
		Old:                   oldSnap,
		Target:                toQuotaPolicySnapshot(chg.Plan.Target),
	}
}
