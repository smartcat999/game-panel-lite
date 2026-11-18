package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/driver"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

const (
	defaultThresholdMemoryRatio  = 85
	defaultThresholdStorageRatio = 85
	defaultThresholdCPURatio     = 90
	defaultThresholdTPSLow       = 15
	defaultThresholdPlayersRatio = 90
	defaultThresholdHeartbeatSec = 40

	defaultForSecondsCPU    = 180
	defaultForSecondsMetric = 120
	defaultAdminTenant      = "admin"
)

// DefaultAlertRules returns the built-in monitoring rules with production-grade pending windows (forSeconds).
func DefaultAlertRules() []corev1.AlertRule {
	return []corev1.AlertRule{
		{ID: "rule-mem-85", Title: "服务器内存使用率预警", Description: "当运行中游戏服务器内存使用率持续达到或超过配额上限的 85% 时触发预警。", Metric: corev1.AlertMetricMemoryUsageRatio, Threshold: defaultThresholdMemoryRatio, ForSeconds: defaultForSecondsMetric, Severity: corev1.AlertSeverityWarning, Scope: corev1.AlertScopeAll, Enabled: true},
		{ID: "rule-disk-85", Title: "服务器磁盘空间告警", Description: "当服务器工作目录磁盘占用持续达到或超过存储配额的 85% 时提醒清理或扩容。", Metric: corev1.AlertMetricStorageUsageRatio, Threshold: defaultThresholdStorageRatio, ForSeconds: defaultForSecondsMetric, Severity: corev1.AlertSeverityWarning, Scope: corev1.AlertScopeAll, Enabled: true},
		{ID: "rule-cpu-90", Title: "服务器 CPU 持续满载预警", Description: "当运行中游戏服务器 CPU 占用率持续达到或超过配额上限的 90% 时触发预警。", Metric: corev1.AlertMetricCPUUsageRatio, Threshold: defaultThresholdCPURatio, ForSeconds: defaultForSecondsCPU, Severity: corev1.AlertSeverityWarning, Scope: corev1.AlertScopeAll, Enabled: true},
		{ID: "rule-tps-low", Title: "游戏服务端 TPS 低帧卡顿预警", Description: "当运行中游戏服务器实时服务端 TPS 持续跌至或低于 15 TPS 时触发卡顿掉帧预警。", Metric: corev1.AlertMetricServerTPSLow, Threshold: defaultThresholdTPSLow, ForSeconds: defaultForSecondsMetric, Severity: corev1.AlertSeverityWarning, Scope: corev1.AlertScopeAll, Enabled: true},
		{ID: "rule-players-90", Title: "游戏在线玩家接近满服提醒", Description: "当运行中游戏服务器在线玩家人数持续达到或超过最大人数上限的 90% 时触发提醒。", Metric: corev1.AlertMetricOnlinePlayersRatio, Threshold: defaultThresholdPlayersRatio, ForSeconds: defaultForSecondsMetric, Severity: corev1.AlertSeverityWarning, Scope: corev1.AlertScopeAll, Enabled: true},
		{ID: "rule-oom-killed", Title: "容器 OOMKilled 异常退出检测", Description: "实时监听容器 ExitCode 137 (OOMKilled) 或进程非零崩溃退出并记录恢复事件。", Metric: corev1.AlertMetricContainerOOMEvents, Threshold: 1, ForSeconds: 0, Severity: corev1.AlertSeverityCritical, Scope: corev1.AlertScopeAll, Enabled: true},
		{ID: "rule-node-heartbeat", Title: "计算节点心跳失联巡检", Description: "周期性检测宿主机节点心跳，超过设定秒数未上报自动标记为 NotReady。", Metric: corev1.AlertMetricNodeHeartbeatAge, Threshold: defaultThresholdHeartbeatSec, ForSeconds: 0, Severity: corev1.AlertSeverityCritical, Scope: corev1.AlertScopePlatform, Enabled: true},
		{ID: "rule-inst-unknown", Title: "服务器所在节点失联检测", Description: "当服务器绑定的计算节点进入 NotReady 状态时，将该节点上的运行服务器置为 Unknown。", Metric: corev1.AlertMetricInstanceNodePhase, Threshold: 1, ForSeconds: 0, Severity: corev1.AlertSeverityCritical, Scope: corev1.AlertScopeAll, Enabled: true},
		{ID: "rule-alertmanager-sync", Title: "Prometheus Alertmanager 外部联动", Description: "自动聚合并同步外部 Alertmanager (/api/v2/alerts) 中处于 active 状态的告警。", Metric: corev1.AlertMetricAlertmanagerV2, Threshold: 1, ForSeconds: 0, Severity: corev1.AlertSeverityWarning, Scope: corev1.AlertScopeAll, Enabled: true},
	}
}

func defaultRuleForSeconds(metric string) int {
	switch metric {
	case corev1.AlertMetricCPUUsageRatio:
		return defaultForSecondsCPU
	case corev1.AlertMetricMemoryUsageRatio,
		corev1.AlertMetricStorageUsageRatio,
		corev1.AlertMetricServerTPSLow,
		corev1.AlertMetricOnlinePlayersRatio,
		corev1.AlertMetricNodeMemoryUsageRatio,
		corev1.AlertMetricNodeDiskUsageRatio:
		return defaultForSecondsMetric
	default:
		return 0
	}
}

// NormalizeAlertRules fills default ID, severity, scope, and metric-appropriate ForSeconds on AlertRule items.
func NormalizeAlertRules(list []corev1.AlertRule) []corev1.AlertRule {
	out := make([]corev1.AlertRule, len(list))
	copy(out, list)
	for i := range out {
		if out[i].ID == "" {
			out[i].ID = fmt.Sprintf("rule-%d", i+1)
		}
		if out[i].Severity == "" {
			out[i].Severity = corev1.AlertSeverityWarning
		}
		if out[i].Scope == "" {
			out[i].Scope = corev1.AlertScopeAll
		}
		if out[i].ForSeconds <= 0 {
			out[i].ForSeconds = defaultRuleForSeconds(out[i].Metric)
		}
	}
	return out
}

func parseAlertRules(annotations map[string]string) []corev1.AlertRule {
	if len(annotations) > 0 {
		if raw := strings.TrimSpace(annotations[corev1.AnnotationPlatformAlertRules]); raw != "" {
			var list []corev1.AlertRule
			if err := json.Unmarshal([]byte(raw), &list); err == nil && len(list) > 0 {
				return NormalizeAlertRules(list)
			}
		}
	}
	return DefaultAlertRules()
}

// ExtractTenantRules returns the configured AlertRules from Tenant.Spec.AlertRules or legacy annotations.
func ExtractTenantRules(t *corev1.Tenant) []corev1.AlertRule {
	if t == nil {
		return DefaultAlertRules()
	}
	if len(t.Spec.AlertRules) > 0 {
		return NormalizeAlertRules(t.Spec.AlertRules)
	}
	return parseAlertRules(t.Metadata.Annotations)
}

// LoadClusterAlertRules loads the active AlertRules from storage (hosted on the admin carrier tenant).
func LoadClusterAlertRules(ctx context.Context, reg *repo.Registry) []corev1.AlertRule {
	if reg == nil || reg.Pool() == nil {
		return DefaultAlertRules()
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	if tenantDB == nil {
		return DefaultAlertRules()
	}
	if adminTenant, err := tenantDB.Get(ctx, defaultAdminTenant); err == nil && adminTenant != nil {
		if len(adminTenant.Spec.AlertRules) > 0 || strings.TrimSpace(adminTenant.Metadata.Annotations[corev1.AnnotationPlatformAlertRules]) != "" {
			return ExtractTenantRules(adminTenant)
		}
	}
	if list, err := tenantDB.List(ctx, db.ListOptions{}); err == nil && list != nil {
		for _, t := range list.Items {
			if t != nil && (len(t.Spec.AlertRules) > 0 || strings.TrimSpace(t.Metadata.Annotations[corev1.AnnotationPlatformAlertRules]) != "") {
				return ExtractTenantRules(t)
			}
		}
	}
	return DefaultAlertRules()
}

func detectChannelType(rawURL string) string {
	return driver.DetectChannelType(rawURL)
}

// ExtractTenantChannels returns the configured workspace AlertChannels from Tenant.Spec.AlertChannels or annotations.
func ExtractTenantChannels(t *corev1.Tenant) []corev1.AlertChannel {
	if t == nil {
		return nil
	}
	if len(t.Spec.AlertChannels) > 0 {
		return normalizeAlertChannels(t.Spec.AlertChannels, "ch-", "默认告警通道")
	}
	return parseTenantChannels(t.Metadata.Annotations)
}

// ExtractPlatformChannels returns the configured platform AlertChannels from Tenant.Spec.PlatformAlertChannels or annotations.
func ExtractPlatformChannels(t *corev1.Tenant) []corev1.AlertChannel {
	if t == nil {
		return nil
	}
	if len(t.Spec.PlatformAlertChannels) > 0 {
		return normalizeAlertChannels(t.Spec.PlatformAlertChannels, "pch-", "平台默认告警通道")
	}
	return parsePlatformChannels(t.Metadata.Annotations)
}

func parseTenantChannels(annotations map[string]string) []corev1.AlertChannel {
	if len(annotations) == 0 {
		return nil
	}
	if raw := strings.TrimSpace(annotations[corev1.AnnotationAlertChannels]); raw != "" {
		if out, ok := unmarshalAlertChannels(raw, "ch-", "默认告警通道"); ok {
			return out
		}
	}
	if legacy := strings.TrimSpace(annotations[corev1.AnnotationAlertWebhook]); legacy != "" {
		return []corev1.AlertChannel{{
			ID:       "default",
			Name:     "默认通知渠道",
			Type:     detectChannelType(legacy),
			URL:      legacy,
			Severity: corev1.AlertScopeAll,
			Enabled:  true,
			State:    corev1.ChannelStateHealthy,
		}}
	}
	return nil
}

func parsePlatformChannels(annotations map[string]string) []corev1.AlertChannel {
	if len(annotations) == 0 {
		return nil
	}
	raw := strings.TrimSpace(annotations[corev1.AnnotationPlatformAlertChannels])
	if raw == "" {
		return nil
	}
	out, _ := unmarshalAlertChannels(raw, "pch-", "平台默认告警通道")
	return out
}

func normalizeAlertChannels(list []corev1.AlertChannel, idPrefix, defaultName string) []corev1.AlertChannel {
	out := make([]corev1.AlertChannel, 0, len(list))
	for i, ch := range list {
		ch.URL = strings.TrimSpace(ch.URL)
		if ch.URL == "" {
			continue
		}
		if ch.ID == "" {
			ch.ID = fmt.Sprintf("%s%d", idPrefix, i+1)
		}
		if ch.Name == "" {
			ch.Name = defaultName
		}
		if ch.Type == "" {
			ch.Type = detectChannelType(ch.URL)
		}
		if ch.Severity == "" {
			ch.Severity = corev1.AlertScopeAll
		}
		if ch.State == "" {
			ch.State = corev1.ChannelStateHealthy
		}
		ch.ConsoleURL = sanitizeConsoleURL(ch.ConsoleURL)
		out = append(out, ch)
	}
	return out
}

func sanitizeConsoleURL(raw string) string {
	return driver.SanitizeConsoleURL(raw)
}

func preserveChannelConsoleURLs(prev, next []corev1.AlertChannel) []corev1.AlertChannel {
	if len(prev) == 0 || len(next) == 0 {
		return next
	}
	byID := make(map[string]string, len(prev))
	for _, c := range prev {
		if clean := sanitizeConsoleURL(c.ConsoleURL); clean != "" && c.ID != "" {
			byID[c.ID] = clean
		}
	}
	for i := range next {
		if next[i].ConsoleURL == "" {
			next[i].ConsoleURL = byID[next[i].ID]
		}
	}
	return next
}

func unmarshalAlertChannels(raw, idPrefix, defaultName string) ([]corev1.AlertChannel, bool) {
	var list []corev1.AlertChannel
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, false
	}
	return normalizeAlertChannels(list, idPrefix, defaultName), true
}

// SyncTenantAlertConfig normalizes and synchronizes declarative Tenant.Spec alert rules/channels/silences with annotations.
func SyncTenantAlertConfig(current, patched *corev1.Tenant) {
	if patched == nil {
		return
	}
	if patched.Metadata.Annotations == nil {
		patched.Metadata.Annotations = make(map[string]string)
	}
	now := time.Now().UTC()
	syncTenantRulesField(current, patched)
	syncPlatformChannelsField(current, patched)
	syncWorkspaceChannelsField(current, patched)
	syncPlatformSilencesField(current, patched, now)
	syncWorkspaceSilencesField(current, patched, now)
	if store := cache.Default(); store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = store.Delete(ctx, cacheKeyAlertSnapshot)
		cancel()
	}
}

func syncTenantRulesField(current, patched *corev1.Tenant) {
	specChanged := current == nil || !jsonEqual(current.Spec.AlertRules, patched.Spec.AlertRules)
	annChanged := current != nil && current.Metadata.Annotations[corev1.AnnotationPlatformAlertRules] != patched.Metadata.Annotations[corev1.AnnotationPlatformAlertRules]
	if specChanged && len(patched.Spec.AlertRules) > 0 {
		patched.Spec.AlertRules = NormalizeAlertRules(patched.Spec.AlertRules)
		if b, err := json.Marshal(patched.Spec.AlertRules); err == nil {
			patched.Metadata.Annotations[corev1.AnnotationPlatformAlertRules] = string(b)
		}
	} else if annChanged || (len(patched.Spec.AlertRules) == 0 && patched.Metadata.Annotations[corev1.AnnotationPlatformAlertRules] != "") {
		patched.Spec.AlertRules = parseAlertRules(patched.Metadata.Annotations)
	}
}

func syncPlatformChannelsField(current, patched *corev1.Tenant) {
	specChanged := current == nil || !jsonEqual(current.Spec.PlatformAlertChannels, patched.Spec.PlatformAlertChannels)
	annChanged := current != nil && current.Metadata.Annotations[corev1.AnnotationPlatformAlertChannels] != patched.Metadata.Annotations[corev1.AnnotationPlatformAlertChannels]
	if specChanged && (patched.Spec.PlatformAlertChannels != nil || current != nil && len(current.Spec.PlatformAlertChannels) > 0) {
		norm := normalizeAlertChannels(patched.Spec.PlatformAlertChannels, "pch-", "平台默认告警通道")
		if current != nil {
			norm = preserveChannelConsoleURLs(ExtractPlatformChannels(current), norm)
		}
		patched.Spec.PlatformAlertChannels = norm
		if b, err := json.Marshal(patched.Spec.PlatformAlertChannels); err == nil {
			patched.Metadata.Annotations[corev1.AnnotationPlatformAlertChannels] = string(b)
		}
	} else if annChanged || (len(patched.Spec.PlatformAlertChannels) == 0 && patched.Metadata.Annotations[corev1.AnnotationPlatformAlertChannels] != "") {
		norm := parsePlatformChannels(patched.Metadata.Annotations)
		if current != nil {
			norm = preserveChannelConsoleURLs(ExtractPlatformChannels(current), norm)
		}
		patched.Spec.PlatformAlertChannels = norm
	}
}

func syncWorkspaceChannelsField(current, patched *corev1.Tenant) {
	specChanged := current == nil || !jsonEqual(current.Spec.AlertChannels, patched.Spec.AlertChannels)
	annChanged := current != nil && current.Metadata.Annotations[corev1.AnnotationAlertChannels] != patched.Metadata.Annotations[corev1.AnnotationAlertChannels]
	if specChanged && (patched.Spec.AlertChannels != nil || current != nil && len(current.Spec.AlertChannels) > 0) {
		norm := normalizeAlertChannels(patched.Spec.AlertChannels, "ch-", "默认告警通道")
		if current != nil {
			norm = preserveChannelConsoleURLs(ExtractTenantChannels(current), norm)
		}
		patched.Spec.AlertChannels = norm
		if b, err := json.Marshal(patched.Spec.AlertChannels); err == nil {
			patched.Metadata.Annotations[corev1.AnnotationAlertChannels] = string(b)
		}
	} else if annChanged || (len(patched.Spec.AlertChannels) == 0 && patched.Metadata.Annotations[corev1.AnnotationAlertChannels] != "") {
		norm := parseTenantChannels(patched.Metadata.Annotations)
		if current != nil {
			norm = preserveChannelConsoleURLs(ExtractTenantChannels(current), norm)
		}
		patched.Spec.AlertChannels = norm
	}
	if _, hasCh := patched.Metadata.Annotations[corev1.AnnotationAlertChannels]; hasCh || len(patched.Spec.AlertChannels) > 0 {
		primaryURL := ""
		for _, ch := range patched.Spec.AlertChannels {
			if ch.Enabled && ch.URL != "" {
				primaryURL = ch.URL
				break
			}
		}
		patched.Metadata.Annotations[corev1.AnnotationAlertWebhook] = primaryURL
	}
}

func syncPlatformSilencesField(current, patched *corev1.Tenant, now time.Time) {
	specChanged := current == nil || !jsonEqual(current.Spec.PlatformAlertSilences, patched.Spec.PlatformAlertSilences)
	annChanged := current != nil && current.Metadata.Annotations[corev1.AnnotationPlatformAlertSilences] != patched.Metadata.Annotations[corev1.AnnotationPlatformAlertSilences]
	if specChanged && (patched.Spec.PlatformAlertSilences != nil || current != nil && len(current.Spec.PlatformAlertSilences) > 0) {
		patched.Spec.PlatformAlertSilences = NormalizeAlertSilences(patched.Spec.PlatformAlertSilences, "psil-", corev1.AlertScopeAll, true, now)
		if b, err := json.Marshal(patched.Spec.PlatformAlertSilences); err == nil {
			patched.Metadata.Annotations[corev1.AnnotationPlatformAlertSilences] = string(b)
		}
	} else if annChanged || (patched.Spec.PlatformAlertSilences == nil && patched.Metadata.Annotations[corev1.AnnotationPlatformAlertSilences] != "") {
		patched.Spec.PlatformAlertSilences = unmarshalSilencesAnnotation(patched.Metadata.Annotations[corev1.AnnotationPlatformAlertSilences], "psil-", corev1.AlertScopeAll, true, now)
	}
}

func syncWorkspaceSilencesField(current, patched *corev1.Tenant, now time.Time) {
	tName := strings.TrimSpace(patched.Metadata.Name)
	specChanged := current == nil || !jsonEqual(current.Spec.AlertSilences, patched.Spec.AlertSilences)
	annChanged := current != nil && current.Metadata.Annotations[corev1.AnnotationAlertSilences] != patched.Metadata.Annotations[corev1.AnnotationAlertSilences]
	if specChanged && (patched.Spec.AlertSilences != nil || current != nil && len(current.Spec.AlertSilences) > 0) {
		patched.Spec.AlertSilences = NormalizeAlertSilences(patched.Spec.AlertSilences, "sil-", tName, false, now)
		if b, err := json.Marshal(patched.Spec.AlertSilences); err == nil {
			patched.Metadata.Annotations[corev1.AnnotationAlertSilences] = string(b)
		}
	} else if annChanged || (patched.Spec.AlertSilences == nil && patched.Metadata.Annotations[corev1.AnnotationAlertSilences] != "") {
		patched.Spec.AlertSilences = unmarshalSilencesAnnotation(patched.Metadata.Annotations[corev1.AnnotationAlertSilences], "sil-", tName, false, now)
	}
}

func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
