package alert

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/alert/providers/feishu"
)

type memKVStore struct {
	mu   sync.Mutex
	data map[string]string
}

func newMemKVStore() *memKVStore {
	return &memKVStore{data: make(map[string]string)}
}

func (m *memKVStore) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[key], nil
}

func (m *memKVStore) Set(_ context.Context, key, val string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = val
	return nil
}

func (m *memKVStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func TestStateMachineTransientSpikeAndHysteresis(t *testing.T) {
	overrideStateStore = newMemKVStore()
	defer func() { overrideStateStore = nil }()

	t0 := time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC)
	spec := transitionSpec{
		forDuration:    3 * time.Minute,
		keepFiringFor:  time.Minute,
		repeatInterval: 4 * time.Hour,
	}
	key := "inst:admin/e2e-tr-vanilla:rule-cpu-90"

	// 1. Reproduce 19s Terraria world-gen CPU spike (98% -> 4%): must emit ZERO transitions.
	b98, r98 := ratioBreachAndRecovery(98, 90)
	if got := evaluateTransition(key, b98, r98, spec, t0); got != "" {
		t.Fatalf("expected initial spike to enter pending silently, got %q", got)
	}
	b4, r4 := ratioBreachAndRecovery(4, 90)
	if got := evaluateTransition(key, b4, r4, spec, t0.Add(19*time.Second)); got != "" {
		t.Fatalf("expected 19s transient spike recovery to reset silently, got %q", got)
	}

	// 2. Sustained breach >= 3m transitions Pending -> Firing once.
	t1 := t0.Add(time.Minute)
	_ = evaluateTransition(key, b98, r98, spec, t1)
	if got := evaluateTransition(key, b98, r98, spec, t1.Add(2*time.Minute)); got != "" {
		t.Fatalf("expected still pending at 2m, got %q", got)
	}
	if got := evaluateTransition(key, b98, r98, spec, t1.Add(3*time.Minute)); got != corev1.AlertStateFiring {
		t.Fatalf("expected firing at 3m, got %q", got)
	}

	// 3. Hysteresis deadband (88% is between 85% and 90%): stays firing without flapping.
	b88, r88 := ratioBreachAndRecovery(88, 90)
	if got := evaluateTransition(key, b88, r88, spec, t1.Add(5*time.Minute)); got != "" {
		t.Fatalf("expected deadband 88%% to stay firing silently, got %q", got)
	}

	// 4. Recovery requires >= 60s steady below 85%.
	tRec := t1.Add(6 * time.Minute)
	if got := evaluateTransition(key, b4, r4, spec, tRec); got != "" {
		t.Fatalf("expected entering recovering window silently, got %q", got)
	}
	if got := evaluateTransition(key, b4, r4, spec, tRec.Add(30*time.Second)); got != "" {
		t.Fatalf("expected still stabilizing at 30s, got %q", got)
	}
	if got := evaluateTransition(key, b4, r4, spec, tRec.Add(60*time.Second)); got != corev1.AlertStateResolved {
		t.Fatalf("expected resolved after 60s steady recovery, got %q", got)
	}
}

func TestRepeatIntervalNodeInhibitionAndSilentClear(t *testing.T) {
	overrideStateStore = newMemKVStore()
	defer func() { overrideStateStore = nil }()

	now := time.Date(2026, 11, 11, 16, 0, 0, 0, time.UTC)
	spec := transitionSpec{forDuration: 0, keepFiringFor: 0, repeatInterval: 4 * time.Hour}

	// Node root-cause alert fires immediately (forDuration=0) and inhibits host checks.
	if got := evaluateTransition("node:phase:um773", true, false, spec, now); got != corev1.AlertStateFiring {
		t.Fatalf("expected node phase alert to fire immediately, got %q", got)
	}
	if !isNodeRootCauseFiring("um773") {
		t.Fatal("expected isNodeRootCauseFiring(um773) to be true")
	}
	if got := evaluateTransition("node:phase:um773", true, false, spec, now.Add(2*time.Hour)); got != "" {
		t.Fatalf("expected no repeat before 4h repeat_interval, got %q", got)
	}
	if got := evaluateTransition("node:phase:um773", true, false, spec, now.Add(4*time.Hour)); got != corev1.AlertStateFiring {
		t.Fatalf("expected repeat notification at 4h, got %q", got)
	}

	// Instance silent cleanup deletes cached state without emitting Resolved.
	instRuleKey := "inst:admin/mc-1:rule-cpu-90"
	_ = evaluateTransition(instRuleKey, true, false, spec, now)
	if firing, _ := isAlertKeyFiring(instRuleKey); !firing {
		t.Fatal("expected instance rule key to be firing before ClearInstanceAlerts")
	}
	ClearInstanceAlerts(context.Background(), nil, "admin", "mc-1")
	if firing, _ := isAlertKeyFiring(instRuleKey); firing {
		t.Fatal("expected ClearInstanceAlerts to clear firing state")
	}
}

func TestStableFingerprintDedupIgnoresDynamicPercentage(t *testing.T) {
	overrideStateStore = newMemKVStore()
	defer func() { overrideStateStore = nil }()

	// Even if CPU changes from 97% to 98%, the stable alertKey is identical and suppressed.
	alertKey := "inst:admin/mc-1:rule-cpu-90"
	if shouldSuppressTransition("tenant:admin", "ch-1", alertKey, corev1.AlertStateFiring) {
		t.Fatal("first firing transition must not be suppressed")
	}
	if !shouldSuppressTransition("tenant:admin", "ch-1", alertKey, corev1.AlertStateFiring) {
		t.Fatal("duplicate firing transition for same alertKey within cooldown must be suppressed")
	}
	if shouldSuppressTransition("tenant:admin", "ch-1", alertKey, corev1.AlertStateResolved) {
		t.Fatal("resolved transition has distinct state and must not be suppressed")
	}
	if shouldSuppressTransition("tenant:admin", "ch-1", alertKey, corev1.AlertStateTest) {
		t.Fatal("channel test probe must never be suppressed")
	}
}

func TestFeishuInteractiveCardBatchDeliveryAndNoDoubleBrackets(t *testing.T) {
	var capturedBody []byte
	var capturedSigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSigHeader = r.Header.Get("X-GamePanel-Signature-256")
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer srv.Close()

	ch := corev1.AlertChannel{
		ID: "ch-feishu", Name: "运维飞书群", Type: corev1.ChannelTypeFeishu,
		URL: srv.URL, Secret: "sec-test-123", ConsoleURL: "http://100.88.0.2:3000", Enabled: true,
	}
	items := []notificationItem{
		{
			AlertKey: "inst:admin/tr-1:rule-cpu-90", Scope: corev1.AlertScopeTenant, Tenant: "admin",
			TargetKind: "instances", TargetName: "tr-1", RuleTitle: "服务器 CPU 持续满载预警",
			Severity: corev1.AlertSeverityWarning, State: corev1.AlertStateFiring,
			MetricValue: "96% (阈值 >= 90%)", Summary: "CPU 持续 3 分钟超阈值。",
		},
		{
			AlertKey: "inst:admin/tr-2:rule-oom-killed", Scope: corev1.AlertScopeTenant, Tenant: "admin",
			TargetKind: "instances", TargetName: "tr-2", RuleTitle: "容器 OOMKilled 异常退出检测",
			Severity: corev1.AlertSeverityCritical, State: corev1.AlertStateFiring,
			MetricValue: "OOMKilled", Summary: "容器发生 ExitCode 137 退出。",
		},
	}
	title, msg, _, _ := summarizeBatchItems(corev1.AlertScopeTenant, "admin", items)
	if err := deliverBatchSync(context.Background(), ch, "dlv-batch-1", title, msg, items); err != nil {
		t.Fatalf("deliverBatchSync failed: %v", err)
	}
	verifyCapturedFeishuPayload(t, capturedBody, capturedSigHeader)
}

func verifyCapturedFeishuPayload(t *testing.T, raw []byte, sigHeader string) {
	t.Helper()
	bodyStr := string(raw)
	if strings.Contains(bodyStr, "[[") || strings.Contains(bodyStr, "]]") {
		t.Fatalf("found double-bracket formatting bug in payload: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "`") {
		t.Fatalf("found raw backtick unsupported by Feishu lark_md in payload: %s", bodyStr)
	}
	if !strings.HasPrefix(sigHeader, "sha256=") {
		t.Fatalf("expected X-GamePanel-Signature-256 header, got %q", sigHeader)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal Feishu payload: %v", err)
	}
	if parsed["msg_type"] != "interactive" {
		t.Fatalf("expected msg_type=interactive, got %v", parsed["msg_type"])
	}
	if parsed["sign"] == "" || parsed["timestamp"] == "" {
		t.Fatalf("expected Feishu HMAC timestamp and sign at root level: %+v", parsed)
	}
	card, _ := parsed["card"].(map[string]any)
	header, _ := card["header"].(map[string]any)
	if header["template"] != "red" {
		t.Fatalf("expected critical batch card template=red, got %v", header["template"])
	}
	headerTitle, _ := header["title"].(map[string]any)
	if got := headerTitle["content"]; got != "工作区 admin · 2 项告警触发" {
		t.Fatalf("expected concise batch header title, got %q", got)
	}
	wantInstanceLink := "http://100.88.0.2:3000/console/servers/tr-1?tenant=admin&tab=console"
	elementsJSON, _ := json.Marshal(card["elements"])
	unescapedElements := strings.ReplaceAll(string(elementsJSON), `\u0026`, "&")
	if !strings.Contains(unescapedElements, wantInstanceLink) || !strings.Contains(unescapedElements, `"tag":"action"`) {
		t.Fatalf("expected inline instance deep link and Feishu action button block in payload: %s", unescapedElements)
	}
	if sanitizeConsoleURL("http://localhost:3000") != "" || sanitizeConsoleURL("http://127.0.0.1:3000") != "" {
		t.Fatal("loopback consoleURL must be rejected so localhost never overwrites public origin")
	}

	testPayload := feishu.BuildCardPayload("工作区运维群", "http://100.88.0.2:3000", []notificationItem{{
		Scope: corev1.AlertScopeTenant, Tenant: "admin", State: corev1.AlertStateTest,
		RuleTitle: "工作区「admin Personal」告警通道测试", Summary: "连通正常。",
	}}, time.Now().UTC(), "1700000000", "")
	testCard, _ := testPayload["card"].(map[string]any)
	testHeader, _ := testCard["header"].(map[string]any)
	testTitle, _ := testHeader["title"].(map[string]any)
	if got := testTitle["content"]; got != "通道连通性测试 · 工作区运维群" {
		t.Fatalf("expected deduplicated test card header title, got %q", got)
	}
}

func TestAlertSilencesMatchExpiryAndIsolation(t *testing.T) {
	now := time.Date(2026, 11, 11, 16, 0, 0, 0, time.UTC)
	rawTenant := []corev1.AlertSilence{
		{ID: "sil-expired", TargetKind: corev1.SilenceTargetInstances, TargetName: "old-mc", ExpiresAt: now.Add(-time.Minute).Format(time.RFC3339)},
		{ID: "sil-node-escape", TargetKind: corev1.SilenceTargetNodes, TargetName: "um773", ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)},
		{ID: "sil-inst-1h", TargetKind: corev1.SilenceTargetInstances, TargetName: "demo-mc-01", Reason: "模组升级", ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)},
		{ID: "sil-rule-4h", TargetKind: corev1.SilenceTargetInstances, TargetName: "demo-mc-02", RuleID: "rule-cpu-90", ExpiresAt: now.Add(4 * time.Hour).Format(time.RFC3339)},
	}
	tSilences := NormalizeAlertSilences(rawTenant, "sil-", "admin", false, now)
	if len(tSilences) != 2 {
		t.Fatalf("expected expired and node-escape workspace silences to be pruned, got %d: %+v", len(tSilences), tSilences)
	}
	pSilences := NormalizeAlertSilences([]corev1.AlertSilence{
		{ID: "psil-node-24h", TargetKind: corev1.SilenceTargetNodes, TargetName: "um773", ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339)},
	}, "psil-", corev1.AlertScopeAll, true, now)

	if m, ok := MatchAlertSilence(tSilences, pSilences, "admin", corev1.SilenceTargetInstances, "demo-mc-01", "rule-oom-killed", now); !ok || m.ID != "sil-inst-1h" {
		t.Fatalf("expected demo-mc-01 to match sil-inst-1h, got ok=%v m=%+v", ok, m)
	}
	if _, ok := MatchAlertSilence(tSilences, pSilences, "studio-alpha", corev1.SilenceTargetInstances, "demo-mc-01", "rule-oom-killed", now); ok {
		t.Fatal("admin workspace silence must never mute studio-alpha instance")
	}
	if _, ok := MatchAlertSilence(tSilences, pSilences, "admin", corev1.SilenceTargetInstances, "demo-mc-02", "rule-mem-85", now); ok {
		t.Fatal("rule-scoped silence (rule-cpu-90) must not mute rule-mem-85")
	}
	if _, ok := MatchAlertSilence(tSilences, pSilences, "admin", corev1.SilenceTargetInstances, "demo-mc-02", "rule-cpu-90", now); !ok {
		t.Fatal("expected rule-scoped silence to mute rule-cpu-90")
	}
	if m, ok := MatchAlertSilence(tSilences, pSilences, "", corev1.SilenceTargetNodes, "um773", "rule-node-heartbeat", now); !ok || m.ID != "psil-node-24h" {
		t.Fatalf("expected platform node silence to match um773, got ok=%v m=%+v", ok, m)
	}
	if _, ok := MatchAlertSilence(tSilences, pSilences, "admin", corev1.SilenceTargetInstances, "demo-mc-01", "rule-oom-killed", now.Add(61*time.Minute)); ok {
		t.Fatal("expected 1h silence to auto-expire at +61m without cron")
	}
	carrier := &corev1.Tenant{}
	carrier.Metadata.Name = "admin"
	carrier.Spec.AlertSilences = tSilences
	if IsAlertEventSilenced(context.Background(), nil, carrier, alertEvent{
		Scope: corev1.AlertScopeTenant, Tenant: "admin", TargetKind: corev1.SilenceTargetInstances,
		TargetName: "demo-mc-01", State: corev1.AlertStateTest,
	}, now) {
		t.Fatal("channel connectivity test (AlertStateTest) must never be muted by silences")
	}

	pruned, changed := PruneTargetSilences([]corev1.AlertSilence{
		{ID: "sil-1", TargetKind: corev1.SilenceTargetInstances, TargetName: "demo-mc-01", ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)},
		{ID: "sil-2", TargetKind: corev1.SilenceTargetInstances, TargetName: "demo-mc-02", ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)},
		{ID: "sil-wild", TargetKind: corev1.SilenceTargetWildcard, TargetName: corev1.SilenceTargetWildcard, ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)},
	}, false, "admin", corev1.SilenceTargetInstances, "demo-mc-01")
	if !changed || len(pruned) != 2 || pruned[0].ID != "sil-2" || pruned[1].ID != "sil-wild" {
		t.Fatalf("expected PruneTargetSilences to remove only demo-mc-01 and keep other/wildcard silences, got changed=%v pruned=%+v", changed, pruned)
	}
}

func TestChannelDriverDetectionAndPayloads(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://open.feishu.cn/open-apis/bot/v2/hook/abc", corev1.ChannelTypeFeishu},
		{"https://open.larksuite.com/open-apis/bot/v2/hook/abc", corev1.ChannelTypeFeishu},
		{"https://oapi.dingtalk.com/robot/send?access_token=abc", corev1.ChannelTypeDingTalk},
		{"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc", corev1.ChannelTypeWeCom},
		{"https://hooks.slack.com/services/T/B/X", corev1.ChannelTypeSlack},
		{"https://discord.com/api/webhooks/1/tok", corev1.ChannelTypeSlack},
		{"https://example.com/webhook", corev1.ChannelTypeWebhook},
		{"telegram://token@telegram?chats=123", corev1.ChannelTypeShoutrrr},
	}
	for _, tc := range cases {
		if got := DetectChannelType(tc.url); got != tc.want {
			t.Fatalf("DetectChannelType(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}

	verifyWebhookDelivery(t, corev1.ChannelTypeDingTalk, "ding-sec", true, func(t *testing.T, query string, body map[string]any) {
		if !strings.Contains(query, "timestamp=") || !strings.Contains(query, "sign=") {
			t.Fatalf("expected DingTalk signed query params, got %q", query)
		}
		if body["msgtype"] != "markdown" {
			t.Fatalf("expected DingTalk msgtype=markdown, got %v", body["msgtype"])
		}
	})
	verifyWebhookDelivery(t, corev1.ChannelTypeWeCom, "", false, func(t *testing.T, _ string, body map[string]any) {
		if body["msgtype"] != "markdown" {
			t.Fatalf("expected WeCom msgtype=markdown, got %v", body["msgtype"])
		}
	})
	verifyWebhookDelivery(t, corev1.ChannelTypeSlack, "", false, func(t *testing.T, _ string, body map[string]any) {
		if body["text"] == "" {
			t.Fatalf("expected Slack text field, got %+v", body)
		}
	})
	verifyWebhookDelivery(t, corev1.ChannelTypeWebhook, "", false, func(t *testing.T, _ string, body map[string]any) {
		if body["deliveryId"] != "dlv-verify" {
			t.Fatalf("expected generic webhook deliveryId, got %+v", body)
		}
	})
}

func verifyWebhookDelivery(t *testing.T, chType, secret string, withQuery bool, assertFn func(*testing.T, string, map[string]any)) {
	t.Helper()
	var rawQuery string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	u := srv.URL
	if withQuery {
		u += "?access_token=abc"
	}
	ch := corev1.AlertChannel{ID: "ch-test", Type: chType, URL: u, Secret: secret, Enabled: true}
	if err := deliverBatchSync(context.Background(), ch, "dlv-verify", "Disk Alert", "95% used", nil); err != nil {
		t.Fatalf("deliverBatchSync(%s) failed: %v", chType, err)
	}
	assertFn(t, rawQuery, body)
}

func TestCustomChannelDriverRegistration(t *testing.T) {
	custom := &testCustomChannelDriver{}
	RegisterChannelDriver(custom)

	if got := DetectChannelType("https://custom.alerts.local/hook"); got != "custom-pager" {
		t.Fatalf("DetectChannelType custom = %q, want custom-pager", got)
	}
	ch := corev1.AlertChannel{ID: "ch-custom", Type: "custom-pager", URL: "https://any.endpoint.local/notify"}
	if err := deliverBatchSync(context.Background(), ch, "dlv-c1", "Node Down", "um773 offline", nil); err != nil {
		t.Fatalf("custom driver delivery failed: %v", err)
	}
	if custom.calls != 1 || custom.lastReq.DeliveryID != "dlv-c1" {
		t.Fatalf("unexpected custom driver capture: calls=%d req=%+v", custom.calls, custom.lastReq)
	}
}

type testCustomChannelDriver struct {
	calls   int
	lastReq DeliveryRequest
}

func (*testCustomChannelDriver) Type() string  { return "custom-pager" }
func (*testCustomChannelDriver) Priority() int { return 5 }
func (*testCustomChannelDriver) MatchURL(u string) bool {
	return strings.Contains(u, "custom.alerts.local")
}
func (d *testCustomChannelDriver) Send(_ context.Context, req DeliveryRequest) error {
	d.calls++
	d.lastReq = req
	return nil
}
