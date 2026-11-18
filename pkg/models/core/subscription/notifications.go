package subscription

import (
	"context"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	alertmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

// EvaluateSubscriptionNotifications dispatches idempotent 4-stage expiry alerts (D-3, D-1, D0, Grace-1d).
func EvaluateSubscriptionNotifications(
	ctx context.Context,
	reg *repo.Registry,
	bus mq.Interface,
	tenant *corev1.Tenant,
	sub corev1.TenantSubscriptionStatus,
	now time.Time,
) (bool, error) {
	if tenant == nil || tenant.Spec.BillingMode != corev1.TenantBillingModeSubscription || sub.ExpiresAt == nil || sub.GraceUntil == nil {
		return false, nil
	}
	stage, title, message, severity, ok := matchNotificationMilestone(*sub.ExpiresAt, *sub.GraceUntil, now.UTC())
	if !ok {
		return false, nil
	}
	return alertmodel.NotifySubscriptionMilestone(ctx, reg, bus, tenant, stage, title, message, severity, *sub.ExpiresAt, now)
}

func matchNotificationMilestone(expiresAt, graceUntil, now time.Time) (stage, title, message, severity string, ok bool) {
	exp := expiresAt.UTC()
	grace := graceUntil.UTC()
	if !now.Before(grace) {
		return "", "", "", "", false
	}
	d3 := exp.Add(-3 * 24 * time.Hour)
	d1 := exp.Add(-24 * time.Hour)
	g1 := grace.Add(-24 * time.Hour)
	switch {
	case !now.Before(g1) && now.Before(grace):
		return corev1.SubscriptionNotifyGraceLast1d,
			"宽限期剩余不足 24 小时",
			"工作区套餐宽限期即将结束，宽限期满后平台管理员将可回收冻结实例，请尽快续期。",
			corev1.AlertSeverityCritical, true
	case !now.Before(exp) && now.Before(g1):
		return corev1.SubscriptionNotifyExpired0d,
			"套餐已到期并进入停服保号宽限期",
			"当前工作区套餐已到期，运行中服务器已安全停服保号，数据与端口在宽限期内完整保留。",
			corev1.AlertSeverityCritical, true
	case !now.Before(d1) && now.Before(exp):
		return corev1.SubscriptionNotifyExpiring1d,
			"套餐将于 24 小时内到期",
			"当前工作区套餐即将到期，到期后服务器将自动停服保号，请及时续期。",
			corev1.AlertSeverityWarning, true
	case !now.Before(d3) && now.Before(d1):
		return corev1.SubscriptionNotifyExpiring3d,
			"套餐将于 3 天内到期",
			"当前工作区套餐将在 3 天内到期，请提前准备续期以免影响游戏服务运行。",
			corev1.AlertSeverityWarning, true
	default:
		return "", "", "", "", false
	}
}
