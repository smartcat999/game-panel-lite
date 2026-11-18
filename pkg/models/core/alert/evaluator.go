package alert

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

type alertEvent struct {
	AlertKey    string
	Scope       string
	Tenant      string
	TargetKind  string
	TargetName  string
	RuleID      string
	RuleTitle   string
	Condition   string
	Severity    string
	State       string
	MetricValue string
	Summary     string
	OccurredAt  time.Time
}

// EvaluateInstanceAlerts evaluates instance phase, resource, and custom game metric alert transitions.
func EvaluateInstanceAlerts(ctx context.Context, reg *repo.Registry, bus mq.Interface, prevPhase corev1.InstancePhase, updated *corev1.Instance) {
	evaluateInstanceAlertsAt(ctx, reg, bus, prevPhase, updated, time.Now().UTC())
}

func evaluateInstanceAlertsAt(ctx context.Context, reg *repo.Registry, bus mq.Interface, prevPhase corev1.InstancePhase, updated *corev1.Instance, now time.Time) {
	if updated == nil || reg == nil || reg.Pool() == nil {
		return
	}
	if shouldSilenceInstanceLifecycle(updated) {
		ClearInstanceAlerts(ctx, reg, updated.Metadata.Tenant, updated.Metadata.Name)
		return
	}
	if isNodeRootCauseFiring(updated.Spec.NodeName) {
		return
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	if tenantDB == nil {
		return
	}
	tObj, err := tenantDB.Get(ctx, updated.Metadata.Tenant)
	if err != nil || tObj == nil {
		return
	}
	evaluateInstancePhaseTransition(ctx, reg, bus, tObj, prevPhase, updated, now)
	if updated.Status.Phase != corev1.InstancePhaseRunning || updated.Status.Usage == nil {
		return
	}
	inGrace := updated.Status.StartedAt != nil && now.Sub(*updated.Status.StartedAt) < startupGracePeriod
	limits := effectiveInstanceLimits(updated)
	for _, r := range LoadClusterAlertRules(ctx, reg) {
		if !ruleAppliesToTenant(r, updated.Metadata.Tenant) {
			continue
		}
		if inGrace && (r.Metric == corev1.AlertMetricCPUUsageRatio || r.Metric == corev1.AlertMetricServerTPSLow) {
			continue
		}
		evaluateInstanceMetricRule(ctx, reg, bus, tObj, updated, r, limits, now)
	}
}

func shouldSilenceInstanceLifecycle(inst *corev1.Instance) bool {
	if inst.Metadata.DeletionTimestamp != nil || inst.Spec.State == corev1.InstanceStateStopped {
		return true
	}
	return inst.Status.Phase == corev1.InstancePhaseStopped || inst.Status.Phase == corev1.InstancePhaseTerminating
}

func effectiveInstanceLimits(inst *corev1.Instance) corev1.ResourceList {
	if len(inst.Spec.Resources.Limits) > 0 {
		return inst.Spec.Resources.Limits
	}
	return inst.Spec.Resources.EffectiveRequests()
}

func evaluateInstancePhaseTransition(ctx context.Context, reg *repo.Registry, bus mq.Interface, tObj *corev1.Tenant, prevPhase corev1.InstancePhase, updated *corev1.Instance, now time.Time) {
	if prevPhase == "" || updated.Status.Phase == prevPhase {
		return
	}
	isUnhealthy := updated.Status.Phase == corev1.InstancePhaseFailed || updated.Status.Phase == corev1.InstancePhaseUnknown
	if !isUnhealthy && updated.Status.Phase != corev1.InstancePhaseRunning {
		return
	}
	tenant, name := updated.Metadata.Tenant, updated.Metadata.Name
	key := fmt.Sprintf("inst:phase:%s/%s", tenant, name)
	spec := transitionSpec{repeatInterval: defaultRepeatInterval}
	state := evaluateTransition(key, isUnhealthy, !isUnhealthy, spec, now)
	if state == "" && activeStateStore() == nil && !isUnhealthy && (prevPhase == corev1.InstancePhaseFailed || prevPhase == corev1.InstancePhaseUnknown) {
		state = corev1.AlertStateResolved
	}
	if state == "" {
		return
	}
	reason, msg := extractInstanceReadyReason(updated)
	ev := alertEvent{
		AlertKey: key, Scope: corev1.AlertScopeTenant, Tenant: tenant,
		TargetKind: "instances", TargetName: name, RuleID: "rule-oom-killed",
		RuleTitle: "服务器运行状态异常", Condition: "phase == " + reason,
		Severity: corev1.AlertSeverityCritical, State: state,
		MetricValue: reason, Summary: msg, OccurredAt: now,
	}
	if state == corev1.AlertStateResolved {
		ev.RuleTitle = "服务器运行状态恢复"
		ev.MetricValue = string(corev1.InstancePhaseRunning)
		ev.Summary = "服务器已恢复正常运行状态。"
	}
	enqueueTenantEvent(ctx, reg, bus, tObj, ev, "")
}

func extractInstanceReadyReason(inst *corev1.Instance) (string, string) {
	reason, msg := string(inst.Status.Phase), "服务器运行状态异常，请检查控制台日志与资源使用情况。"
	for _, c := range inst.Status.Conditions {
		if c.Type == metav1.ConditionTypeReady {
			if c.Reason != "" {
				reason = c.Reason
			}
			if c.Message != "" {
				msg = c.Message
			}
			break
		}
	}
	return reason, msg
}

func evaluateInstanceMetricRule(ctx context.Context, reg *repo.Registry, bus mq.Interface, tObj *corev1.Tenant, inst *corev1.Instance, r corev1.AlertRule, limits corev1.ResourceList, now time.Time) {
	switch r.Metric {
	case corev1.AlertMetricMemoryUsageRatio:
		evalInstanceRatio(ctx, reg, bus, tObj, inst, r, inst.Status.Usage.MemoryBytes, limits.MemoryBytes(), defaultThresholdMemoryRatio, "内存使用率", "memory_usage", now)
	case corev1.AlertMetricStorageUsageRatio:
		evalInstanceRatio(ctx, reg, bus, tObj, inst, r, inst.Status.Usage.StorageBytes, limits.StorageBytes(), defaultThresholdStorageRatio, "磁盘占用率", "disk_usage", now)
	case corev1.AlertMetricCPUUsageRatio:
		evalInstanceRatio(ctx, reg, bus, tObj, inst, r, inst.Status.Usage.CPUMillicores, limits.CPUMillicores(), defaultThresholdCPURatio, "CPU 使用率", "cpu_usage", now)
	case corev1.AlertMetricOnlinePlayersRatio, corev1.AlertMetricServerTPSLow:
		evalInstanceCustomMetric(ctx, reg, bus, tObj, inst, r, now)
	}
}

func evalInstanceRatio(ctx context.Context, reg *repo.Registry, bus mq.Interface, tObj *corev1.Tenant, inst *corev1.Instance, r corev1.AlertRule, used, limit int64, defTh int, label, condPrefix string, now time.Time) {
	if limit <= 0 {
		return
	}
	pct := int(float64(used) * 100 / float64(limit))
	th := defaultPositive(r.Threshold, defTh)
	breached, recovered := ratioBreachAndRecovery(pct, th)
	tenant, name := inst.Metadata.Tenant, inst.Metadata.Name
	key := fmt.Sprintf("inst:%s/%s:%s", tenant, name, r.ID)
	spec := ruleTransitionSpec(r)
	state := evaluateTransition(key, breached, recovered, spec, now)
	if state == "" {
		return
	}
	cond := fmt.Sprintf("%s >= %d%%", condPrefix, th)
	valText := fmt.Sprintf("%d%% (阈值 >= %d%%)", pct, th)
	summary := r.Description
	if state == corev1.AlertStateResolved {
		valText = fmt.Sprintf("%d%% (阈值 < %d%%)", pct, th)
		summary = fmt.Sprintf("服务器%s已平稳回落至安全范围（当前 %d%%）。", label, pct)
	}
	enqueueTenantEvent(ctx, reg, bus, tObj, alertEvent{
		AlertKey: key, Scope: corev1.AlertScopeTenant, Tenant: tenant,
		TargetKind: "instances", TargetName: name, RuleID: r.ID, RuleTitle: r.Title,
		Condition: cond, Severity: r.Severity, State: state,
		MetricValue: valText, Summary: summary, OccurredAt: now,
	}, "")
}

func evalInstanceCustomMetric(ctx context.Context, reg *repo.Registry, bus mq.Interface, tObj *corev1.Tenant, inst *corev1.Instance, r corev1.AlertRule, now time.Time) {
	for _, cm := range inst.Status.Usage.CustomMetrics {
		if r.Metric == corev1.AlertMetricOnlinePlayersRatio && cm.ID == corev1.CustomMetricIDOnlinePlayers {
			evalOnlinePlayersMetric(ctx, reg, bus, tObj, inst, r, cm, now)
		} else if r.Metric == corev1.AlertMetricServerTPSLow && cm.ID == corev1.CustomMetricIDServerTPS && cm.Value > 0 {
			evalServerTPSMetric(ctx, reg, bus, tObj, inst, r, cm, now)
		}
	}
}

func evalOnlinePlayersMetric(ctx context.Context, reg *repo.Registry, bus mq.Interface, tObj *corev1.Tenant, inst *corev1.Instance, r corev1.AlertRule, cm corev1.InstanceCustomMetric, now time.Time) {
	maxPlayers, _ := strconv.ParseFloat(strings.TrimSpace(cm.Max), 64)
	if maxPlayers <= 0 {
		return
	}
	pct := int(cm.Value * 100 / maxPlayers)
	th := defaultPositive(r.Threshold, defaultThresholdPlayersRatio)
	breached, recovered := ratioBreachAndRecovery(pct, th)
	tenant, name := inst.Metadata.Tenant, inst.Metadata.Name
	key := fmt.Sprintf("inst:%s/%s:%s", tenant, name, r.ID)
	state := evaluateTransition(key, breached, recovered, ruleTransitionSpec(r), now)
	if state == "" {
		return
	}
	valText := fmt.Sprintf("%s/%s 人 (%d%%)", cm.Display, cm.Max, pct)
	summary := r.Description
	if state == corev1.AlertStateResolved {
		summary = fmt.Sprintf("在线玩家人数已回落至 %s/%s 人（%d%%）。", cm.Display, cm.Max, pct)
	}
	enqueueTenantEvent(ctx, reg, bus, tObj, alertEvent{
		AlertKey: key, Scope: corev1.AlertScopeTenant, Tenant: tenant,
		TargetKind: "instances", TargetName: name, RuleID: r.ID, RuleTitle: r.Title,
		Condition: fmt.Sprintf("online_players >= %d%%", th), Severity: r.Severity, State: state,
		MetricValue: valText, Summary: summary, OccurredAt: now,
	}, "")
}

func evalServerTPSMetric(ctx context.Context, reg *repo.Registry, bus mq.Interface, tObj *corev1.Tenant, inst *corev1.Instance, r corev1.AlertRule, cm corev1.InstanceCustomMetric, now time.Time) {
	th := defaultPositive(r.Threshold, defaultThresholdTPSLow)
	breached, recovered := tpsBreachAndRecovery(cm.Value, th)
	tenant, name := inst.Metadata.Tenant, inst.Metadata.Name
	key := fmt.Sprintf("inst:%s/%s:%s", tenant, name, r.ID)
	state := evaluateTransition(key, breached, recovered, ruleTransitionSpec(r), now)
	if state == "" {
		return
	}
	valText := fmt.Sprintf("%s TPS (阈值 <= %d)", cm.Display, th)
	summary := r.Description
	if state == corev1.AlertStateResolved {
		summary = fmt.Sprintf("服务端 TPS 已稳定恢复至 %s TPS。", cm.Display)
	}
	enqueueTenantEvent(ctx, reg, bus, tObj, alertEvent{
		AlertKey: key, Scope: corev1.AlertScopeTenant, Tenant: tenant,
		TargetKind: "instances", TargetName: name, RuleID: r.ID, RuleTitle: r.Title,
		Condition: fmt.Sprintf("server_tps <= %d TPS", th), Severity: r.Severity, State: state,
		MetricValue: valText, Summary: summary, OccurredAt: now,
	}, "")
}

// NotifyNodeHeartbeatExpired records and dispatches a platform-scoped Critical alert when a Node enters NotReady.
func NotifyNodeHeartbeatExpired(ctx context.Context, reg *repo.Registry, bus mq.Interface, node *corev1.Node) {
	if node == nil || reg == nil || reg.Pool() == nil {
		return
	}
	now := time.Now().UTC()
	key := "node:phase:" + node.Metadata.Name
	state := evaluateTransition(key, true, false, transitionSpec{repeatInterval: defaultRepeatInterval}, now)
	if state != corev1.AlertStateFiring {
		return
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	if tenantDB == nil {
		return
	}
	enqueueAllPlatformCarriers(ctx, reg, tenantDB, bus, alertEvent{
		AlertKey: key, Scope: corev1.AlertScopePlatform,
		TargetKind: "nodes", TargetName: node.Metadata.Name, RuleID: "rule-node-heartbeat",
		RuleTitle: "计算节点心跳失联", Condition: "node.phase == NotReady",
		Severity: corev1.AlertSeverityCritical, State: corev1.AlertStateFiring,
		MetricValue: string(corev1.NodePhaseNotReady),
		Summary:     "宿主机心跳超时未上报，请检查节点网络连通性与 node-agent 服务状态。",
		OccurredAt:  now,
	})
}

// EvaluateNodeAlerts evaluates node phase recovery, clock skew, and host memory/disk threshold transitions.
func EvaluateNodeAlerts(ctx context.Context, reg *repo.Registry, bus mq.Interface, prevPhase corev1.NodePhase, updated *corev1.Node) {
	if updated == nil || reg == nil || reg.Pool() == nil {
		return
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	if tenantDB == nil {
		return
	}
	now := time.Now().UTC()
	name := updated.Metadata.Name
	clockResolved := evaluateNodeClockAlert(ctx, reg, tenantDB, bus, updated, now)
	if updated.Status.Phase == corev1.NodePhaseReady && !clockResolved {
		key := "node:phase:" + name
		state := evaluateTransition(key, false, true, transitionSpec{}, now)
		if state == "" && activeStateStore() == nil && prevPhase == corev1.NodePhaseNotReady {
			state = corev1.AlertStateResolved
		}
		if state == corev1.AlertStateResolved {
			enqueueAllPlatformCarriers(ctx, reg, tenantDB, bus, alertEvent{
				AlertKey: key, Scope: corev1.AlertScopePlatform,
				TargetKind: "nodes", TargetName: name, RuleID: "rule-node-heartbeat",
				RuleTitle: "计算节点心跳恢复", Condition: "node.phase == NotReady",
				Severity: corev1.AlertSeverityCritical, State: corev1.AlertStateResolved,
				MetricValue: string(corev1.NodePhaseReady),
				Summary:     "宿主机节点心跳已恢复正常上报，节点重回 Ready 就绪状态。",
				OccurredAt:  now,
			})
		}
	}
	if updated.Status.Phase != corev1.NodePhaseReady {
		return
	}
	capMem, usedMem, capDisk, usedDisk := extractNodeMemoryAndDiskUsage(updated)
	for _, r := range LoadClusterAlertRules(ctx, reg) {
		if !r.Enabled {
			continue
		}
		switch r.Metric {
		case corev1.AlertMetricNodeMemoryUsageRatio:
			evalNodeRatio(ctx, reg, tenantDB, bus, name, r, usedMem, capMem, defaultThresholdMemoryRatio, "物理内存使用率", "node_memory", now)
		case corev1.AlertMetricNodeDiskUsageRatio:
			evalNodeRatio(ctx, reg, tenantDB, bus, name, r, usedDisk, capDisk, defaultThresholdStorageRatio, "磁盘空间使用率", "node_disk", now)
		}
	}
}

func evaluateNodeClockAlert(ctx context.Context, reg *repo.Registry, tenantDB *tenantdb.DB, bus mq.Interface, updated *corev1.Node, now time.Time) bool {
	clockCond := metav1.GetCondition(updated.Status.Conditions, metav1.ConditionTypeClockSynced)
	if clockCond == nil {
		return false
	}
	name := updated.Metadata.Name
	skewed := clockCond.Status == metav1.ConditionFalse
	key := "node:clock:" + name
	state := evaluateTransition(key, skewed, !skewed, transitionSpec{repeatInterval: defaultRepeatInterval}, now)
	if state == "" {
		return false
	}
	msg := clockCond.Message
	if msg == "" {
		msg = "宿主机节点与控制面时钟偏差超过 30 秒，请执行 timedatectl set-ntp true 同步时钟。"
	}
	val := "ClockSkew > 30s"
	if state == corev1.AlertStateResolved {
		msg = "宿主机节点与控制面时钟已恢复同步（偏差 <= 30s）。"
		val = "ClockSynced"
	}
	enqueueAllPlatformCarriers(ctx, reg, tenantDB, bus, alertEvent{
		AlertKey: key, Scope: corev1.AlertScopePlatform,
		TargetKind: "nodes", TargetName: name, RuleID: "rule-node-clock",
		RuleTitle: "计算节点时钟同步检测", Condition: "clock_skew > 30s",
		Severity: corev1.AlertSeverityWarning, State: state,
		MetricValue: val, Summary: msg, OccurredAt: now,
	})
	return state == corev1.AlertStateResolved
}

func extractNodeMemoryAndDiskUsage(updated *corev1.Node) (int64, int64, int64, int64) {
	capMem := updated.Status.Capacity.MemoryBytes()
	usedMem := capMem - updated.Status.Allocatable.MemoryBytes()
	if updated.Status.Usage != nil && updated.Status.Usage.MemoryBytes > 0 {
		usedMem = updated.Status.Usage.MemoryBytes
	}
	capDisk := updated.Status.Capacity.StorageBytes()
	usedDisk := capDisk - updated.Status.Allocatable.StorageBytes()
	if updated.Status.Usage != nil && updated.Status.Usage.StorageBytes > 0 {
		usedDisk = updated.Status.Usage.StorageBytes
	}
	return capMem, usedMem, capDisk, usedDisk
}

func evalNodeRatio(ctx context.Context, reg *repo.Registry, tenantDB *tenantdb.DB, bus mq.Interface, name string, r corev1.AlertRule, used, capBytes int64, defTh int, label, condPrefix string, now time.Time) {
	if capBytes <= 0 || used < 0 || used > capBytes {
		return
	}
	pct := int(float64(used) * 100 / float64(capBytes))
	th := defaultPositive(r.Threshold, defTh)
	breached, recovered := ratioBreachAndRecovery(pct, th)
	key := fmt.Sprintf("node:%s:%s", name, r.ID)
	state := evaluateTransition(key, breached, recovered, ruleTransitionSpec(r), now)
	if state == "" {
		return
	}
	valText := fmt.Sprintf("%d%% (阈值 >= %d%%)", pct, th)
	summary := r.Description
	if state == corev1.AlertStateResolved {
		valText = fmt.Sprintf("%d%% (阈值 < %d%%)", pct, th)
		summary = fmt.Sprintf("宿主机%s已平稳回落至安全范围（当前 %d%%）。", label, pct)
	}
	enqueueAllPlatformCarriers(ctx, reg, tenantDB, bus, alertEvent{
		AlertKey: key, Scope: corev1.AlertScopePlatform,
		TargetKind: "nodes", TargetName: name, RuleID: r.ID, RuleTitle: r.Title,
		Condition: fmt.Sprintf("%s >= %d%%", condPrefix, th), Severity: r.Severity, State: state,
		MetricValue: valText, Summary: summary, OccurredAt: now,
	})
}

func ruleTransitionSpec(r corev1.AlertRule) transitionSpec {
	forSec := r.ForSeconds
	if forSec <= 0 {
		forSec = defaultRuleForSeconds(r.Metric)
	}
	keepFiring := defaultKeepFiringDuration
	if forSec <= 0 {
		keepFiring = 0
	}
	return transitionSpec{
		forDuration:    time.Duration(forSec) * time.Second,
		keepFiringFor:  keepFiring,
		repeatInterval: defaultRepeatInterval,
	}
}

// EnqueueChangedWebhookTests checks whether workspace or platform webhook test annotations changed
// on a Tenant update/patch and dispatches an immediate test notification (zero group_wait delay).
func EnqueueChangedWebhookTests(ctx context.Context, reg *repo.Registry, bus mq.Interface, prevAnns map[string]string, updated *corev1.Tenant) {
	if updated == nil {
		return
	}
	now := time.Now().UTC()
	if raw := updated.Metadata.Annotations[corev1.AnnotationAlertWebhookTest]; raw != "" && raw != prevAnns[corev1.AnnotationAlertWebhookTest] {
		chID := parseTestChannelTarget(raw)
		displayName := strings.TrimSpace(updated.Spec.DisplayName)
		if displayName == "" {
			displayName = updated.Metadata.Name
		}
		enqueueTenantEvent(ctx, reg, bus, updated, alertEvent{
			AlertKey: "test:tenant:" + updated.Metadata.Name, Scope: corev1.AlertScopeTenant, Tenant: updated.Metadata.Name,
			TargetKind: "channels", TargetName: displayName, RuleTitle: fmt.Sprintf("工作区「%s」告警通道测试", displayName),
			Severity: corev1.AlertSeverityCritical, State: corev1.AlertStateTest,
			MetricValue: "连通性正常",
			Summary:     "这是一条来自 GamePanel Lite 工作区的测试告警通知，说明您的工作区告警接收通道配置正常。",
			OccurredAt:  now,
		}, chID)
	}
	if raw := updated.Metadata.Annotations[corev1.AnnotationPlatformAlertWebhookTest]; raw != "" && raw != prevAnns[corev1.AnnotationPlatformAlertWebhookTest] {
		chID := parseTestChannelTarget(raw)
		enqueuePlatformEvent(ctx, reg, bus, updated, alertEvent{
			AlertKey: "test:platform:" + updated.Metadata.Name, Scope: corev1.AlertScopePlatform,
			TargetKind: "channels", TargetName: "平台全局", RuleTitle: "平台集群全局告警通道测试",
			Severity: corev1.AlertSeverityCritical, State: corev1.AlertStateTest,
			MetricValue: "连通性正常",
			Summary:     "这是一条来自 GamePanel Lite 平台管理后台的测试告警通知，说明您的集群级告警接收通道配置正常。",
			OccurredAt:  now,
		}, chID)
	}
}

func parseTestChannelTarget(raw string) string {
	if idx := strings.Index(raw, "|"); idx > 0 {
		return strings.TrimSpace(raw[:idx])
	}
	return ""
}
