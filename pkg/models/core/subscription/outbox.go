package subscription

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
)

const billingOutboxBatchSize = 50

// FlushTenantBillingOutbox delivers unexported policy changes and applied orders to billing.v1.metering idempotently.
func FlushTenantBillingOutbox(ctx context.Context, reg *repo.Registry, bus mq.Interface, tenant *corev1.Tenant) error {
	if bus == nil || reg == nil || reg.Pool() == nil || tenant == nil || tenant.Metadata.UID == "" {
		return nil
	}
	quotaDB := quotadb.NewDB(reg.Pool())
	if err := flushPolicyChangesOutbox(ctx, quotaDB, bus, tenant); err != nil {
		return err
	}
	return flushAppliedOrdersOutbox(ctx, quotaDB, bus, tenant)
}

func flushPolicyChangesOutbox(ctx context.Context, quotaDB *quotadb.DB, bus mq.Interface, tenant *corev1.Tenant) error {
	changes, err := quotaDB.ListUnexportedPolicyChanges(ctx, tenant.Metadata.UID, billingOutboxBatchSize)
	if err != nil || len(changes) == 0 {
		return err
	}
	for _, chg := range changes {
		ev := buildPolicyChangedCloudEvent(tenant, chg)
		if err := bus.Publish(ctx, mq.TopicMetering, ev); err != nil {
			return err
		}
		if err := quotaDB.MarkPolicyChangeExported(ctx, chg.Plan.ChangeID, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func buildPolicyChangedCloudEvent(tenant *corev1.Tenant, chg quotadb.PolicyChange) mq.Event {
	quotas, unlimited := corev1.FormatLimits(chg.Plan.Target.Limits)
	quotaRaw, _ := json.Marshal(quotas)
	var expStr string
	if chg.Plan.Target.ExpiresAt != nil {
		expStr = chg.Plan.Target.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	attrs := map[string]string{
		"tenantUID":      tenant.Metadata.UID,
		"changeID":       chg.Plan.ChangeID,
		"changeKind":     string(chg.Plan.Kind),
		"fromVersion":    strconv.FormatInt(chg.Plan.FromVersion, 10),
		"toVersion":      strconv.FormatInt(chg.Plan.ToVersion, 10),
		"orderIDs":       strings.Join(chg.Plan.OrderIDs, ","),
		"expiresAt":      expStr,
		"quotaJSON":      string(quotaRaw),
		"unlimitedCount": strconv.Itoa(len(unlimited)),
	}
	return mq.NewMeteringCloudEventAt(chg.Plan.ChangeID, corev1.CloudEventQuotaPolicyChanged, tenant.Metadata.Name, tenant.Metadata.Name, chg.CreatedAt, attrs)
}

func flushAppliedOrdersOutbox(ctx context.Context, quotaDB *quotadb.DB, bus mq.Interface, tenant *corev1.Tenant) error {
	orders, err := quotaDB.ListUnexportedAppliedOrders(ctx, tenant.Metadata.UID, billingOutboxBatchSize)
	if err != nil || len(orders) == 0 {
		return err
	}
	for _, ord := range orders {
		ev := buildOrderAppliedCloudEvent(tenant, ord)
		if err := bus.Publish(ctx, mq.TopicMetering, ev); err != nil {
			return err
		}
		if err := quotaDB.MarkOrderExported(ctx, ord.ID, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func buildOrderAppliedCloudEvent(tenant *corev1.Tenant, ord corev1.QuotaOrder) mq.Event {
	var expStr string
	if ord.ExpiresAt != nil {
		expStr = ord.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	ts := ord.UpdatedAt
	if ord.AppliedAt != nil {
		ts = *ord.AppliedAt
	}
	deltaRaw, _ := json.Marshal(ord.QuotaDelta)
	attrs := map[string]string{
		"tenantUID":     ord.TenantUID,
		"orderID":       ord.ID,
		"orderKind":     ord.OrderKind,
		"planName":      ord.PlanName,
		"source":        ord.Source,
		"durationDays":  strconv.Itoa(ord.DurationDays),
		"amountCents":   strconv.FormatInt(ord.AmountCents, 10),
		"currency":      ord.Currency,
		"policyVersion": strconv.FormatInt(ord.PolicyVersion, 10),
		"expiresAt":     expStr,
		"quotaDelta":    string(deltaRaw),
	}
	return mq.NewMeteringCloudEventAt(ord.ID, corev1.CloudEventOrderApplied, tenant.Metadata.Name, ord.PlanName, ts, attrs)
}
