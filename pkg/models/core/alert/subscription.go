package alert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	auditdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/audit"
)

// NotifySubscriptionMilestone records an idempotent subscription milestone event once per expiry cycle
// and dispatches it to any configured Tenant alert channels. Returns inserted=true on first delivery.
func NotifySubscriptionMilestone(
	ctx context.Context,
	reg *repo.Registry,
	bus mq.Interface,
	tenant *corev1.Tenant,
	stage, title, message, severity string,
	expiresAt, now time.Time,
) (bool, error) {
	if reg == nil || reg.Pool() == nil || tenant == nil || tenant.Metadata.UID == "" {
		return false, nil
	}
	ts := now.UTC()
	expKey := expiresAt.UTC().Format(time.RFC3339)
	idKey := tenant.Metadata.UID + ":" + expKey + ":" + stage
	sum := sha256.Sum256([]byte(idKey))
	eventID := "sub-ntf-" + hex.EncodeToString(sum[:12])
	ev := buildSubscriptionMilestoneAuditEvent(eventID, idKey, tenant, stage, title, message, severity, ts)
	inserted, err := auditdb.NewDB(reg.Pool()).RecordOnce(ctx, ev)
	if err != nil || !inserted {
		return inserted, err
	}
	enqueueSubscriptionChannels(ctx, reg, bus, tenant, idKey, stage, title, message, severity, ts)
	return true, nil
}

func buildSubscriptionMilestoneAuditEvent(
	eventID, idKey string,
	tenant *corev1.Tenant,
	stage, title, message, severity string,
	ts time.Time,
) *corev1.Event {
	attrs := map[string]string{
		"tenant":         tenant.Metadata.Name,
		"tenantUID":      tenant.Metadata.UID,
		"scope":          corev1.AlertScopeTenant,
		"severity":       severity,
		"state":          corev1.AlertStateFiring,
		"alertKey":       "subscription:" + tenant.Metadata.UID + ":" + stage,
		"ruleID":         "subscription-" + stage,
		"ruleTitle":      title,
		"targetKind":     corev1.AlertTargetKindSubscription,
		"targetName":     tenant.Metadata.Name,
		"title":          title,
		"message":        message,
		"idempotencyKey": idKey,
	}
	return &corev1.Event{
		TypeMeta:     metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "Event"},
		Metadata:     metav1.ObjectMeta{Name: eventID, UID: eventID, Tenant: tenant.Metadata.Name, CreationTimestamp: ts},
		Actor:        iamv1.PrincipalSubscriptionReconciler,
		Verb:         iamv1.VerbCreate,
		APIGroup:     corev1.GroupName,
		Resource:     corev1.ResourceAlerts,
		Subresource:  corev1.SubresourceState,
		ResourceName: eventID,
		Action:       corev1.AlertActionTransition,
		TargetName:   tenant.Metadata.Name,
		StatusCode:   200,
		ClientIP:     "127.0.0.1",
		RequestID:    eventID,
		Timestamp:    ts,
		Attributes:   attrs,
	}
}

func enqueueSubscriptionChannels(
	ctx context.Context,
	reg *repo.Registry,
	bus mq.Interface,
	tenant *corev1.Tenant,
	idKey, stage, title, message, severity string,
	ts time.Time,
) {
	channels := ExtractTenantChannels(tenant)
	if len(channels) == 0 {
		return
	}
	item := toNotificationItem(alertEvent{
		AlertKey:   idKey,
		Scope:      corev1.AlertScopeTenant,
		Tenant:     tenant.Metadata.Name,
		TargetKind: corev1.AlertTargetKindSubscription,
		TargetName: tenant.Metadata.Name,
		RuleID:     "subscription-" + stage,
		RuleTitle:  title,
		Severity:   severity,
		State:      corev1.AlertStateFiring,
		Summary:    message,
		OccurredAt: ts,
	})
	for _, ch := range channels {
		if !channelMatchesFilter(ch, severity, corev1.AlertStateFiring, "") {
			continue
		}
		if !publishOrBatchChannelItem(ctx, reg, bus, tenant.Metadata.Name, tenant.Metadata.UID, corev1.AlertScopeTenant, ch, item) {
			dispatchInMemoryFallback(ch, corev1.AlertScopeTenant, tenant.Metadata.Name, item)
		}
	}
}
