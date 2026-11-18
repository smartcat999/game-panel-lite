package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	auditdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/audit"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
)

// Operator provides read-side queries for active and historical cluster/workspace Alerts.
type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Alert], error)
}

type amAlertItem struct {
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`
	StartsAt    time.Time         `json:"startsAt"`
	Status      struct {
		State string `json:"state"`
	} `json:"status"`
}

const (
	alertmanagerActiveAlertsPath = "/api/v2/alerts?active=true"
	defaultAlertPollInterval     = 10 * time.Second
	defaultAlertHTTPTimeout      = 3 * time.Second
	defaultAlertEvalTimeout      = 5 * time.Second
	cacheKeyAMActive             = "alert:am:active"
	cacheKeyAlertSnapshot        = "alert:snapshot:all"
)

var amRestyClient = resty.New().SetTimeout(defaultAlertHTTPTimeout)

func loadAlertmanagerAlerts(ctx context.Context, amURL string) []amAlertItem {
	if amURL == "" {
		return nil
	}
	store := cache.Default()
	if store != nil {
		if raw, err := store.Get(ctx, cacheKeyAMActive); err == nil && raw != "" {
			var cached []amAlertItem
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return cached
			}
		}
	}
	var list []amAlertItem
	reqURL := strings.TrimRight(amURL, "/") + alertmanagerActiveAlertsPath
	resp, err := amRestyClient.R().SetContext(ctx).SetResult(&list).Get(reqURL)
	if err != nil || resp == nil || resp.StatusCode() != http.StatusOK {
		return nil
	}
	if store != nil {
		if encoded, err := json.Marshal(list); err == nil {
			_ = store.Set(ctx, cacheKeyAMActive, string(encoded), defaultAlertPollInterval)
		}
	}
	return list
}

// RefreshAlertSnapshotAsync triggers a non-blocking background evaluation of cluster/tenant alerts
// and updates Prometheus active alert gauges and the unified cache snapshot.
func RefreshAlertSnapshotAsync(reg *repo.Registry) {
	if reg == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultAlertEvalTimeout)
		defer cancel()
		op := NewOperator(reg).(*operator)
		all := op.evaluateAllAlerts(ctx)
		recordAlertPrometheusGauges(all)
		if store := cache.Default(); store != nil {
			if encoded, err := json.Marshal(all); err == nil {
				_ = store.Set(ctx, cacheKeyAlertSnapshot, string(encoded), defaultAlertEvalTimeout)
			}
		}
	}()
}

func recordAlertPrometheusGauges(alerts []*corev1.Alert) {
	metrics.ActiveAlertsGauge.Reset()
	for _, a := range alerts {
		if a == nil || a.State != corev1.AlertStateFiring {
			continue
		}
		tName := a.Metadata.Tenant
		if tName == "" {
			tName = "_platform"
		}
		sev := a.Severity
		if sev == "" {
			sev = corev1.AlertSeverityWarning
		}
		res := a.Resource
		if res == "" {
			res = "instances"
		}
		metrics.ActiveAlertsGauge.WithLabelValues(tName, sev, res).Inc()
	}
}

type operator struct {
	nodeDB     *nodedb.DB
	instanceDB *instancedb.DB
	auditStore *auditdb.DB
	reg        *repo.Registry
}

// NewOperator creates a read-side Alert Operator backed by the repository Registry.
func NewOperator(reg *repo.Registry) Operator {
	op := &operator{reg: reg}
	if reg != nil {
		op.nodeDB = nodedb.NewDB(reg.Pool())
		op.instanceDB = instancedb.NewDB(reg.Pool())
		op.auditStore = auditdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Alert], error) {
	items := o.getFilteredAlertsFromSnapshot(ctx, opts)
	return &metav1.List[*corev1.Alert]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "AlertList"},
		Metadata: metav1.ListMeta{TotalCount: len(items)},
		Items:    items,
	}, nil
}

func (o *operator) getFilteredAlertsFromSnapshot(ctx context.Context, opts db.ListOptions) []*corev1.Alert {
	var all []*corev1.Alert
	if o.reg != nil {
		store := cache.Default()
		var loaded bool
		if store != nil {
			if raw, err := store.Get(ctx, cacheKeyAlertSnapshot); err == nil && raw != "" {
				if json.Unmarshal([]byte(raw), &all) == nil {
					loaded = true
				}
			}
		}
		if !loaded {
			all = o.evaluateAllAlerts(ctx)
			recordAlertPrometheusGauges(all)
			if store != nil {
				if encoded, err := json.Marshal(all); err == nil {
					_ = store.Set(ctx, cacheKeyAlertSnapshot, string(encoded), defaultAlertEvalTimeout)
				}
			}
		}
		all = overlayActiveSilences(ctx, o.reg, all, time.Now().UTC())
	}
	return filterAndPaginateAlerts(all, opts)
}

func filterAndPaginateAlerts(all []*corev1.Alert, opts db.ListOptions) []*corev1.Alert {
	tenantFilter := strings.TrimSpace(opts.Tenant)
	var out []*corev1.Alert
	for _, a := range all {
		if a == nil {
			continue
		}
		if tenantFilter != "" && a.Metadata.Tenant != tenantFilter {
			continue
		}
		if matchesAlertSelectors(a, opts) {
			out = append(out, a)
		}
	}
	if opts.Offset > 0 {
		if opts.Offset >= len(out) {
			return []*corev1.Alert{}
		}
		out = out[opts.Offset:]
	}
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	if out == nil {
		out = []*corev1.Alert{}
	}
	return out
}

func formatBytesGiB(b int64) string {
	const gib = float64(1024 * 1024 * 1024)
	const mib = float64(1024 * 1024)
	if float64(b) >= gib {
		return fmt.Sprintf("%.1f GiB", float64(b)/gib)
	}
	return fmt.Sprintf("%.0f MiB", float64(b)/mib)
}

func matchesAlertSelectors(a *corev1.Alert, opts db.ListOptions) bool {
	if opts.LabelSelector != nil && !opts.LabelSelector.Matches(a.Metadata.Labels) {
		return false
	}
	for k, v := range opts.FieldSelector {
		switch k {
		case "metadata.name":
			if a.Metadata.Name != v {
				return false
			}
		case "metadata.tenant":
			if a.Metadata.Tenant != v {
				return false
			}
		case "spec.severity", "severity":
			if a.Severity != v {
				return false
			}
		case "spec.state", "state":
			if a.State != v {
				return false
			}
		case "spec.scope", "scope":
			if a.Scope != v {
				return false
			}
		case "spec.resource", "resource":
			if a.Resource != v {
				return false
			}
		case "spec.targetName", "targetName":
			if a.TargetName != v {
				return false
			}
		}
	}
	return true
}

func ruleAppliesToTenant(r corev1.AlertRule, tenantName string) bool {
	if !r.Enabled {
		return false
	}
	scope := strings.TrimSpace(r.Scope)
	if scope == "" || scope == corev1.AlertScopeAll || scope == corev1.AlertScopePlatform {
		return true
	}
	return scope == tenantName
}

func (o *operator) evaluateAllAlerts(ctx context.Context) []*corev1.Alert {
	rules := LoadClusterAlertRules(ctx, o.reg)
	seen := make(map[string]bool)
	var out []*corev1.Alert

	add := func(a *corev1.Alert) {
		if a == nil {
			return
		}
		a.DefaultMeta()
		if seen[a.Metadata.Name] {
			return
		}
		seen[a.Metadata.Name] = true
		out = append(out, a)
	}

	evaluateAlertmanagerRules(ctx, rules, add)
	if o.reg != nil {
		now := time.Now().UTC()
		o.evaluatePlatformNodeRules(ctx, rules, now, add)
		o.evaluateTenantInstanceRules(ctx, rules, now, add)
		o.evaluateHistoricalAuditAlerts(ctx, rules, out, add)
	}
	if out == nil {
		out = []*corev1.Alert{}
	}
	return out
}

func (o *operator) evaluateHistoricalAuditAlerts(ctx context.Context, rules []corev1.AlertRule, active []*corev1.Alert, add func(*corev1.Alert)) {
	if o.auditStore == nil {
		return
	}
	list, err := o.auditStore.List(ctx, db.ListOptions{
		Limit:          150,
		SkipTotalCount: true,
		FieldSelector:  map[string]string{"resource": "alerts"},
	})
	if err != nil || list == nil {
		return
	}
	activeTargets := make(map[string]bool, len(active))
	for _, a := range active {
		if a != nil {
			activeTargets[a.Resource+":"+a.Metadata.Tenant+":"+a.TargetName] = true
		}
	}
	for _, ev := range list.Items {
		if a := convertAuditEventToAlert(ev, rules, activeTargets); a != nil {
			add(a)
		}
	}
}

func convertAuditEventToAlert(ev *corev1.Event, rules []corev1.AlertRule, activeTargets map[string]bool) *corev1.Alert {
	if ev == nil || ev.Attributes == nil {
		return nil
	}
	attrs := ev.Attributes
	rawState := strings.TrimSpace(attrs["state"])
	if rawState != corev1.AlertStateFiring && rawState != corev1.AlertStateResolved {
		return nil
	}
	res, tenant, target, ruleID, cond, cleanTitle := extractStructuredOrLegacyAuditMeta(ev, rules)
	if target == "" || activeTargets[res+":"+tenant+":"+target] {
		return nil
	}
	scope := corev1.AlertScopeTenant
	if res == "nodes" {
		scope = corev1.AlertScopePlatform
		tenant = ""
	}
	sev := strings.TrimSpace(attrs["severity"])
	if sev == "" {
		sev = corev1.AlertSeverityWarning
	}
	summary := strings.TrimSpace(attrs["message"])
	if summary == "" {
		summary = cleanTitle
	}
	name := fmt.Sprintf("hist-%s-%s-%s-%s", res, ruleID, tenant, target)
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = ev.Metadata.CreationTimestamp
	}
	return &corev1.Alert{
		Metadata:   metav1.ObjectMeta{Name: name, UID: name, Tenant: tenant, CreationTimestamp: ts, Labels: map[string]string{"ruleID": ruleID}},
		Title:      cleanTitle,
		Summary:    summary,
		Severity:   sev,
		State:      corev1.AlertStateResolved,
		Scope:      scope,
		Resource:   res,
		TargetName: target,
		Value:      "已恢复正常",
		Condition:  cond,
		StartsAt:   ts,
	}
}

func extractStructuredOrLegacyAuditMeta(ev *corev1.Event, rules []corev1.AlertRule) (res, tenant, target, ruleID, cond, cleanTitle string) {
	attrs := ev.Attributes
	if kind := strings.TrimSpace(attrs["targetKind"]); (kind == "instances" || kind == "nodes") && strings.TrimSpace(attrs["targetName"]) != "" {
		res = kind
		target = strings.TrimSpace(attrs["targetName"])
		tenant = strings.TrimSpace(ev.Metadata.Tenant)
		if tenant == "" {
			tenant = strings.TrimSpace(attrs["tenant"])
		}
		ruleID = strings.TrimSpace(attrs["ruleID"])
		if ruleID == "" {
			ruleID = "rule-custom"
		}
		cond = strings.TrimSpace(attrs["condition"])
		ruleTitle := strings.TrimSpace(attrs["ruleTitle"])
		if res == "nodes" {
			return res, "", target, ruleID, cond, fmt.Sprintf("计算节点 %s · %s", target, ruleTitle)
		}
		return res, tenant, target, ruleID, cond, fmt.Sprintf("服务器 %s · %s", target, ruleTitle)
	}
	return parseAuditAlertTargetAndRule(ev.Metadata.Tenant, attrs["tenant"], strings.TrimSpace(attrs["title"]), rules)
}

func parseAuditAlertTargetAndRule(metaTenant, attrTenant, rawTitle string, rules []corev1.AlertRule) (res, tenant, target, ruleID, cond, cleanTitle string) {
	if rawTitle == "" {
		return "", "", "", "", "", ""
	}
	title := rawTitle
	for _, prefix := range []string{"[集群告警 Firing]", "[告警 Firing]", "[已恢复 Resolved]", "[告警触发]", "[已恢复]"} {
		title = strings.TrimSpace(strings.TrimPrefix(title, prefix))
	}
	tenant = strings.TrimSpace(metaTenant)
	if tenant == "" {
		tenant = strings.TrimSpace(attrTenant)
	}
	if strings.HasPrefix(title, "计算节点 ") {
		parts := strings.Fields(strings.TrimPrefix(title, "计算节点 "))
		if len(parts) > 0 {
			res, target = "nodes", parts[0]
		}
		ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricNodeHeartbeatAge, "rule-node-heartbeat", "heartbeat_age > 40s", rules)
		cleanTitle = fmt.Sprintf("计算节点 %s 失联", target)
		if strings.Contains(title, "内存") {
			ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricNodeMemoryUsageRatio, "rule-node-mem", "node_memory >= 85%", rules)
			cleanTitle = title
		} else if strings.Contains(title, "磁盘") {
			ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricNodeDiskUsageRatio, "rule-node-disk", "node_disk >= 85%", rules)
			cleanTitle = title
		}
		return res, "", target, ruleID, cond, cleanTitle
	}
	if strings.HasPrefix(title, "工作区 ") {
		parts := strings.Fields(strings.ReplaceAll(strings.TrimPrefix(title, "工作区 "), "·", " "))
		if len(parts) >= 3 && (parts[1] == "服务器" || parts[1] == "实例") {
			tenant, res, target = parts[0], "instances", parts[2]
		}
	}
	if target == "" {
		return "", "", "", "", "", ""
	}
	ruleID, cond, cleanTitle = matchInstanceAuditRule(title, target, rules)
	return res, tenant, target, ruleID, cond, cleanTitle
}

func matchInstanceAuditRule(title, target string, rules []corev1.AlertRule) (ruleID, cond, cleanTitle string) {
	switch {
	case strings.Contains(title, "内存") && strings.Contains(title, "%"):
		ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricMemoryUsageRatio, "rule-mem-85", "memory_usage >= 85%", rules)
		return ruleID, cond, fmt.Sprintf("服务器 %s 内存使用率预警", target)
	case strings.Contains(title, "磁盘"):
		ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricStorageUsageRatio, "rule-disk-85", "disk_usage >= 85%", rules)
		return ruleID, cond, fmt.Sprintf("服务器 %s 磁盘空间告警", target)
	case strings.Contains(title, "CPU"):
		ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricCPUUsageRatio, "rule-cpu-90", "cpu_usage >= 90%", rules)
		return ruleID, cond, fmt.Sprintf("服务器 %s CPU 持续满载预警", target)
	case strings.Contains(title, "玩家"):
		ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricOnlinePlayersRatio, "rule-players-90", "online_players >= 90%", rules)
		return ruleID, cond, fmt.Sprintf("服务器 %s 在线玩家接近满服提醒", target)
	case strings.Contains(title, "TPS"):
		ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricServerTPSLow, "rule-tps-low", "server_tps <= 15 TPS", rules)
		return ruleID, cond, fmt.Sprintf("服务器 %s 服务端 TPS 低帧卡顿预警", target)
	case strings.Contains(title, "失联") || strings.Contains(title, "Unknown"):
		ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricInstanceNodePhase, "rule-inst-unknown", "node.phase == NotReady", rules)
		return ruleID, cond, fmt.Sprintf("服务器 %s 所在节点失联", target)
	default:
		ruleID, cond = resolveAuditRuleMeta(corev1.AlertMetricContainerOOMEvents, "rule-oom-killed", "reason == OOMKilled", rules)
		return ruleID, cond, fmt.Sprintf("服务器 %s 运行状态异常", target)
	}
}

func resolveAuditRuleMeta(metric, fallbackID, fallbackCond string, rules []corev1.AlertRule) (string, string) {
	for _, r := range rules {
		if r.Metric == metric {
			return r.ID, fallbackCond
		}
	}
	return fallbackID, fallbackCond
}

func evaluateAlertmanagerRules(ctx context.Context, rules []corev1.AlertRule, add func(*corev1.Alert)) {
	amEnabled := false
	for _, r := range rules {
		if r.Enabled && r.Metric == corev1.AlertMetricAlertmanagerV2 {
			amEnabled = true
			break
		}
	}
	amURL := strings.TrimSpace(config.Get().Observability.AlertmanagerURL)
	if !amEnabled || amURL == "" {
		return
	}
	for _, a := range loadAlertmanagerAlerts(ctx, amURL) {
		if a.Status.State == "active" {
			add(convertAMAlertItem(a))
		}
	}
}

func convertAMAlertItem(a amAlertItem) *corev1.Alert {
	alertName := a.Labels["alertname"]
	tName := a.Labels["tenant"]
	target := a.Labels["instance"]
	if target == "" {
		target = a.Labels["node"]
	}
	res, scope := "instances", corev1.AlertScopeTenant
	if tName == "" {
		res, scope = "nodes", corev1.AlertScopePlatform
	}
	sev := a.Labels["severity"]
	if sev == "" {
		sev = corev1.AlertSeverityWarning
	}
	title := a.Annotations["summary"]
	if title == "" {
		title = alertName
	}
	desc := a.Annotations["description"]
	if desc == "" {
		desc = title
	}
	name := fmt.Sprintf("am-%s-%s-%s", strings.ToLower(alertName), tName, target)
	return &corev1.Alert{
		Metadata:   metav1.ObjectMeta{Name: name, UID: name, Tenant: tName, CreationTimestamp: a.StartsAt, Labels: a.Labels, Annotations: a.Annotations},
		Title:      title,
		Summary:    desc,
		Severity:   sev,
		State:      corev1.AlertStateFiring,
		Scope:      scope,
		Resource:   res,
		TargetName: target,
		Value:      a.Annotations["value"],
		Condition:  alertName,
		StartsAt:   a.StartsAt,
	}
}

func (o *operator) evaluatePlatformNodeRules(ctx context.Context, rules []corev1.AlertRule, now time.Time, add func(*corev1.Alert)) {
	if o.nodeDB == nil {
		return
	}
	nodes, err := o.nodeDB.List(ctx, db.ListOptions{})
	if err != nil || nodes == nil {
		return
	}
	for _, n := range nodes.Items {
		if n == nil {
			continue
		}
		for _, r := range rules {
			if r.Enabled {
				add(evaluateSingleNodeRule(n, r, now))
			}
		}
	}
}

func evaluateSingleNodeRule(n *corev1.Node, r corev1.AlertRule, now time.Time) *corev1.Alert {
	switch r.Metric {
	case corev1.AlertMetricNodeHeartbeatAge:
		thresholdSec := defaultPositive(r.Threshold, defaultThresholdHeartbeatSec)
		expired := n.Status.Phase == corev1.NodePhaseNotReady
		if !expired && n.Status.LastHeartbeatTime != nil {
			expired = now.Sub(*n.Status.LastHeartbeatTime) > time.Duration(thresholdSec)*time.Second
		}
		if !expired {
			return nil
		}
		ts := n.Metadata.CreationTimestamp
		if n.Status.LastHeartbeatTime != nil {
			ts = *n.Status.LastHeartbeatTime
		}
		return buildNodeAlert(fmt.Sprintf("node-hb-%s-%s", r.ID, n.Metadata.Name), r.ID, n.Metadata.Name,
			fmt.Sprintf("计算节点 %s 失联 (%s)", n.Metadata.Name, r.Title), r.Description, r.Severity,
			fmt.Sprintf("心跳超时 > %ds", thresholdSec), fmt.Sprintf("heartbeat_age > %ds", thresholdSec), ts)
	case corev1.AlertMetricNodeMemoryUsageRatio:
		return evaluateNodeUsageRatio(n.Metadata.Name, "node-mem", "物理内存", "node_memory",
			n.Status.Capacity.MemoryBytes(), n.Status.Allocatable.MemoryBytes(), r, now)
	case corev1.AlertMetricNodeDiskUsageRatio:
		return evaluateNodeUsageRatio(n.Metadata.Name, "node-disk", "磁盘", "node_disk",
			n.Status.Capacity.StorageBytes(), n.Status.Allocatable.StorageBytes(), r, now)
	}
	return nil
}

func evaluateNodeUsageRatio(nodeName, prefix, label, metric string, capBytes, allocBytes int64, r corev1.AlertRule, now time.Time) *corev1.Alert {
	thresholdPct := defaultPositive(r.Threshold, defaultThresholdMemoryRatio)
	if capBytes <= 0 || allocBytes < 0 || allocBytes > capBytes {
		return nil
	}
	used := capBytes - allocBytes
	pct := int(float64(used) * 100 / float64(capBytes))
	if !isMetricSnapshotFiring(fmt.Sprintf("node:%s:%s", nodeName, r.ID), pct >= thresholdPct, r) {
		return nil
	}
	return buildNodeAlert(fmt.Sprintf("%s-%s-%s", prefix, r.ID, nodeName), r.ID, nodeName,
		fmt.Sprintf("计算节点 %s %s使用率达 %d%%", nodeName, label, pct), r.Description, r.Severity,
		fmt.Sprintf("%s / %s (%d%%)", formatBytesGiB(used), formatBytesGiB(capBytes), pct),
		fmt.Sprintf("%s >= %d%%", metric, thresholdPct), now)
}

func buildNodeAlert(name, ruleID, target, title, summary, severity, value, condition string, ts time.Time) *corev1.Alert {
	return &corev1.Alert{
		Metadata:   metav1.ObjectMeta{Name: name, UID: name, CreationTimestamp: ts, Labels: map[string]string{"ruleID": ruleID}},
		Title:      title,
		Summary:    summary,
		Severity:   severity,
		State:      corev1.AlertStateFiring,
		Scope:      corev1.AlertScopePlatform,
		Resource:   "nodes",
		TargetName: target,
		Value:      value,
		Condition:  condition,
		StartsAt:   ts,
	}
}

func (o *operator) evaluateTenantInstanceRules(ctx context.Context, rules []corev1.AlertRule, now time.Time, add func(*corev1.Alert)) {
	if o.instanceDB == nil {
		return
	}
	instList, err := o.instanceDB.List(ctx, db.ListOptions{})
	if err != nil || instList == nil {
		return
	}
	for _, inst := range instList.Items {
		if inst == nil || shouldSilenceInstanceLifecycle(inst) {
			continue
		}
		condReason, condMsg, condTime := readyConditionDetails(inst)
		for _, r := range rules {
			if ruleAppliesToTenant(r, inst.Metadata.Tenant) {
				add(evaluateSingleInstanceRule(inst, r, condReason, condMsg, condTime, now))
			}
		}
	}
}

func readyConditionDetails(inst *corev1.Instance) (reason, msg string, ts time.Time) {
	for _, c := range inst.Status.Conditions {
		if c.Type == metav1.ConditionTypeReady {
			reason, msg, ts = c.Reason, c.Message, c.LastTransitionTime
			break
		}
	}
	if ts.IsZero() {
		ts = inst.Metadata.CreationTimestamp
	}
	return reason, msg, ts
}

func evaluateSingleInstanceRule(inst *corev1.Instance, r corev1.AlertRule, condReason, condMsg string, condTime, now time.Time) *corev1.Alert {
	inGrace := inst.Status.StartedAt != nil && now.Sub(*inst.Status.StartedAt) < startupGracePeriod
	if inGrace && (r.Metric == corev1.AlertMetricCPUUsageRatio || r.Metric == corev1.AlertMetricServerTPSLow) {
		return nil
	}
	switch r.Metric {
	case corev1.AlertMetricContainerOOMEvents, corev1.AlertMetricInstanceNodePhase:
		return evaluateInstanceHealthRule(inst, r, condReason, condMsg, condTime)
	case corev1.AlertMetricMemoryUsageRatio, corev1.AlertMetricStorageUsageRatio, corev1.AlertMetricCPUUsageRatio:
		return evaluateInstanceResourceRule(inst, r, now)
	case corev1.AlertMetricOnlinePlayersRatio, corev1.AlertMetricServerTPSLow:
		return evaluateInstanceCustomMetricRule(inst, r, now)
	}
	return nil
}

func evaluateInstanceHealthRule(inst *corev1.Instance, r corev1.AlertRule, condReason, condMsg string, condTime time.Time) *corev1.Alert {
	tName, iName := inst.Metadata.Tenant, inst.Metadata.Name
	if r.Metric == corev1.AlertMetricInstanceNodePhase {
		if inst.Status.Phase != corev1.InstancePhaseUnknown {
			return nil
		}
		return buildInstanceAlert(fmt.Sprintf("inst-unknown-%s-%s-%s", r.ID, tName, iName), r.ID, tName, iName,
			fmt.Sprintf("服务器 %s 所在节点失联", iName), fmt.Sprintf("宿主机节点 %s 失联，无法获取实时运行状态。", inst.Spec.NodeName),
			r.Severity, corev1.AlertStateFiring, "节点连接中断", "node.phase == NotReady", condTime)
	}
	if condReason == corev1.ReasonOOMKilled || inst.Status.Phase == corev1.InstancePhaseFailed {
		title, val, cond := fmt.Sprintf("服务器 %s 运行异常", iName), "启动或运行异常", "phase == Failed"
		if condReason == corev1.ReasonOOMKilled {
			title, val, cond = fmt.Sprintf("服务器 %s 内存溢出异常退出", iName), "内存超限退出 (OOM)", "reason == OOMKilled"
		}
		if condMsg == "" {
			condMsg = r.Description
		}
		return buildInstanceAlert(fmt.Sprintf("inst-failed-%s-%s-%s", r.ID, tName, iName), r.ID, tName, iName,
			title, condMsg, r.Severity, corev1.AlertStateFiring, val, cond, condTime)
	}
	if condReason == "OOMRecovered" {
		return buildInstanceAlert(fmt.Sprintf("inst-oomrecovered-%s-%s-%s", r.ID, tName, iName), r.ID, tName, iName,
			fmt.Sprintf("服务器 %s 曾发生内存不足并自动恢复", iName), condMsg, corev1.AlertSeverityWarning, corev1.AlertStateResolved, "已自动恢复运行", "reason == OOMKilled", condTime)
	}
	return nil
}

func evaluateInstanceResourceRule(inst *corev1.Instance, r corev1.AlertRule, now time.Time) *corev1.Alert {
	if inst.Status.Phase != corev1.InstancePhaseRunning || inst.Status.Usage == nil {
		return nil
	}
	limits := effectiveInstanceLimits(inst)
	tName, iName := inst.Metadata.Tenant, inst.Metadata.Name
	switch r.Metric {
	case corev1.AlertMetricMemoryUsageRatio:
		return checkInstanceByteRatio(tName, iName, "inst-mem", "内存", "memory_usage", inst.Status.Usage.MemoryBytes, limits.MemoryBytes(), r, now)
	case corev1.AlertMetricStorageUsageRatio:
		return checkInstanceByteRatio(tName, iName, "inst-disk", "磁盘", "disk_usage", inst.Status.Usage.StorageBytes, limits.StorageBytes(), r, now)
	case corev1.AlertMetricCPUUsageRatio:
		cpuLimit := limits.CPUMillicores()
		thresholdPct := defaultPositive(r.Threshold, defaultThresholdCPURatio)
		if cpuLimit <= 0 {
			return nil
		}
		pct := int(float64(inst.Status.Usage.CPUMillicores) * 100 / float64(cpuLimit))
		if !isMetricSnapshotFiring(fmt.Sprintf("inst:%s/%s:%s", tName, iName, r.ID), pct >= thresholdPct, r) {
			return nil
		}
		return buildInstanceAlert(fmt.Sprintf("inst-cpu-%s-%s-%s", r.ID, tName, iName), r.ID, tName, iName,
			fmt.Sprintf("服务器 %s CPU 使用率达 %d%% (%s)", iName, pct, r.Title), r.Description, r.Severity, corev1.AlertStateFiring,
			fmt.Sprintf("%dm / %dm (%d%%)", inst.Status.Usage.CPUMillicores, cpuLimit, pct), fmt.Sprintf("cpu_usage >= %d%%", thresholdPct), now)
	}
	return nil
}

func checkInstanceByteRatio(tName, iName, prefix, label, condKey string, usedBytes, limitBytes int64, r corev1.AlertRule, now time.Time) *corev1.Alert {
	thresholdPct := defaultPositive(r.Threshold, defaultThresholdMemoryRatio)
	if limitBytes <= 0 {
		return nil
	}
	pct := int(float64(usedBytes) * 100 / float64(limitBytes))
	if !isMetricSnapshotFiring(fmt.Sprintf("inst:%s/%s:%s", tName, iName, r.ID), pct >= thresholdPct, r) {
		return nil
	}
	return buildInstanceAlert(fmt.Sprintf("%s-%s-%s-%s", prefix, r.ID, tName, iName), r.ID, tName, iName,
		fmt.Sprintf("服务器 %s %s使用率达 %d%% (%s)", iName, label, pct, r.Title), r.Description, r.Severity, corev1.AlertStateFiring,
		fmt.Sprintf("%s / %s (%d%%)", formatBytesGiB(usedBytes), formatBytesGiB(limitBytes), pct), fmt.Sprintf("%s >= %d%%", condKey, thresholdPct), now)
}

func evaluateInstanceCustomMetricRule(inst *corev1.Instance, r corev1.AlertRule, now time.Time) *corev1.Alert {
	if inst.Status.Phase != corev1.InstancePhaseRunning || inst.Status.Usage == nil {
		return nil
	}
	tName, iName := inst.Metadata.Tenant, inst.Metadata.Name
	key := fmt.Sprintf("inst:%s/%s:%s", tName, iName, r.ID)
	for _, cm := range inst.Status.Usage.CustomMetrics {
		if r.Metric == corev1.AlertMetricOnlinePlayersRatio && cm.ID == corev1.CustomMetricIDOnlinePlayers {
			maxPlayers, _ := strconv.ParseFloat(strings.TrimSpace(cm.Max), 64)
			thresholdPct := defaultPositive(r.Threshold, defaultThresholdPlayersRatio)
			if maxPlayers > 0 && cm.Value > 0 {
				pct := int(cm.Value * 100 / maxPlayers)
				if isMetricSnapshotFiring(key, pct >= thresholdPct, r) {
					return buildInstanceAlert(fmt.Sprintf("inst-players-%s-%s-%s", r.ID, tName, iName), r.ID, tName, iName,
						fmt.Sprintf("服务器 %s 在线玩家达 %s/%s 人 (%d%%)", iName, cm.Display, cm.Max, pct), r.Description, r.Severity, corev1.AlertStateFiring,
						fmt.Sprintf("%s / %s 人 (%d%%)", cm.Display, cm.Max, pct), fmt.Sprintf("online_players >= %d%%", thresholdPct), now)
				}
			}
		} else if r.Metric == corev1.AlertMetricServerTPSLow && cm.ID == corev1.CustomMetricIDServerTPS && cm.Value > 0 {
			th := defaultPositive(r.Threshold, defaultThresholdTPSLow)
			if isMetricSnapshotFiring(key, cm.Value <= float64(th), r) {
				return buildInstanceAlert(fmt.Sprintf("inst-tps-%s-%s-%s", r.ID, tName, iName), r.ID, tName, iName,
					fmt.Sprintf("服务器 %s 服务端 TPS 跌至 %s (%s)", iName, cm.Display, r.Title), r.Description, r.Severity, corev1.AlertStateFiring,
					fmt.Sprintf("%s TPS (<= %d)", cm.Display, th), fmt.Sprintf("server_tps <= %d TPS", th), now)
			}
		}
	}
	return nil
}

func isMetricSnapshotFiring(alertKey string, rawBreached bool, r corev1.AlertRule) bool {
	if r.ForSeconds <= 0 {
		return rawBreached
	}
	firing, tracked := isAlertKeyFiring(alertKey)
	if !tracked {
		return rawBreached
	}
	return firing
}

func buildInstanceAlert(name, ruleID, tenant, target, title, summary, severity, state, value, condition string, ts time.Time) *corev1.Alert {
	return &corev1.Alert{
		Metadata: metav1.ObjectMeta{Name: name, UID: name, Tenant: tenant, CreationTimestamp: ts, Labels: map[string]string{"ruleID": ruleID}},
		Title:    title, Summary: summary, Severity: severity, State: state, Scope: corev1.AlertScopeTenant,
		Resource: "instances", TargetName: target, Value: value, Condition: condition, StartsAt: ts,
	}
}

func defaultPositive(val, fallback int) int {
	if val <= 0 {
		return fallback
	}
	return val
}
