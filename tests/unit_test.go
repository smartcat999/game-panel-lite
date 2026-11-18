package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/network"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
	alertmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert"
	nodemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/node"
)

func TestConfigDefaultsAndValidation(t *testing.T) {
	cfg := config.New()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}

	badCache := &config.CacheConfig{Driver: "memory"}
	if err := badCache.Validate(); err == nil {
		t.Fatal("expected unsupported cache.driver 'memory' to be rejected")
	}

	redisCache := &config.CacheConfig{Driver: "redis", Addr: "127.0.0.1:6379"}
	if err := redisCache.Validate(); err != nil {
		t.Fatalf("expected redis cache config to validate: %v", err)
	}

	badMQ := &config.MQConfig{Driver: "memory"}
	if err := badMQ.Validate(); err == nil {
		t.Fatal("expected unsupported mq.driver 'memory' to be rejected")
	}

	natsMQ := &config.MQConfig{Driver: "nats", Addr: "nats://127.0.0.1:4222"}
	natsMQ.Default()
	if err := natsMQ.Validate(); err != nil {
		t.Fatalf("expected nats mq config to validate: %v", err)
	}

	for _, rel := range []string{
		"../configs/apiserver.yaml",
		"../configs/controller-manager.yaml",
		"../configs/node-agent.yaml",
		"../configs/config.example.yaml",
	} {
		loaded, err := config.LoadFile(rel)
		if err != nil {
			t.Fatalf("LoadFile(%s): %v", rel, err)
		}
		if loaded.Network.PortRangeMin != 20000 || loaded.Network.PortRangeMax != 29999 {
			t.Fatalf("LoadFile(%s): unexpected network port range %+v", rel, loaded.Network)
		}
	}
}

func TestLabelSelectorMatching(t *testing.T) {
	sel, err := metav1.ParseLabelSelector("region=ap-east-1,tier in (standard,pro),!deprecated")
	if err != nil {
		t.Fatalf("ParseLabelSelector: %v", err)
	}
	if !sel.Matches(map[string]string{"region": "ap-east-1", "tier": "pro"}) {
		t.Fatal("expected labels to match selector")
	}
	if sel.Matches(map[string]string{"region": "ap-east-1", "tier": "pro", "deprecated": "true"}) {
		t.Fatal("expected deprecated label to fail selector")
	}
}

func TestQuantityAndResourceControlMath(t *testing.T) {
	rl := corev1.ResourceList{
		corev1.ResourceCPU:     "2000m",
		corev1.ResourceMemory:  "4Gi",
		corev1.ResourceStorage: "20Gi",
	}
	if got := rl.CPUMillicores(); got != 2000 {
		t.Fatalf("expected 2000m CPU, got %d", got)
	}
	if got := rl.MemoryBytes(); got != 4<<30 {
		t.Fatalf("expected 4Gi memory, got %d", got)
	}

	a := corev1.Resources{CPUMillicores: 1000, MemoryBytes: 1 << 30, StorageBytes: 10 << 30, InstanceCount: 1}
	b := corev1.Resources{CPUMillicores: 2000, MemoryBytes: 2 << 30, StorageBytes: 10 << 30, InstanceCount: 1}
	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Resources.Add: %v", err)
	}
	if sum.CPUMillicores != 3000 || sum.InstanceCount != 2 {
		t.Fatalf("unexpected sum: %+v", sum)
	}
}

func TestInfraPluginsNetworkVolumeAndS3(t *testing.T) {
	netDrv, err := network.Open(&config.NetworkConfig{
		Driver: "hostport", PortRangeMin: 25000, PortRangeMax: 25010, MaxPortProbes: 16,
	})
	if err != nil || netDrv.Handler() != "hostport" {
		t.Fatalf("network.Open: drv=%v err=%v", netDrv, err)
	}
	claimed := map[int32]bool{}
	assigned, err := netDrv.AssignPorts("attempt-1", []corev1.PortRequest{
		{Name: "game", Protocol: "TCP", TargetPort: 25565, PublishedPort: 0},
		{Name: "rcon", Protocol: "TCP", TargetPort: 25575, PublishedPort: 25005},
	}, func(_ corev1.PortRequest, port int32) (bool, error) {
		if claimed[port] {
			return false, nil
		}
		claimed[port] = true
		return true, nil
	})
	if err != nil || len(assigned) != 2 || !netDrv.PortRange().Contains(assigned[0].PublishedPort) || assigned[1].PublishedPort != 25005 {
		t.Fatalf("AssignPorts: assigned=%+v err=%v", assigned, err)
	}

	volDrv, err := volume.NewDriver("hostpath", t.TempDir())
	if err != nil || volDrv.Handler() != "hostpath" {
		t.Fatalf("volume.NewDriver: drv=%v err=%v", volDrv, err)
	}
	hostDir, err := volDrv.ResolveWorkDir("uid-1", "default", "mc-1", true)
	if err != nil || !strings.HasSuffix(hostDir, "/instances/uid-1") {
		t.Fatalf("ResolveWorkDir: dir=%q err=%v", hostDir, err)
	}
	token, err := volume.SignFileTicket("secret-1", volume.FileTicketClaims{
		Tenant: "default", Instance: "mc-1", Action: "read", Path: "/server.properties",
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("SignFileTicket: %v", err)
	}
	if _, err := volume.VerifyFileTicket("secret-1", token, time.Now()); err != nil {
		t.Fatalf("VerifyFileTicket: %v", err)
	}

	s3Cli := s3.NewClient(&config.S3Config{
		Endpoint: "http://127.0.0.1:9000", Region: "us-east-1", Bucket: "backups",
		AccessKey: "ak", SecretKey: "sk", UsePathStyle: true, PresignTTL: 10 * time.Minute,
	}, nil)
	presigned, _, err := s3Cli.PresignPut(context.Background(), "tenants/default/instances/mc-1/b1.tar.gz")
	if err != nil || !strings.Contains(presigned, "X-Amz-Signature=") {
		t.Fatalf("s3Cli.PresignPut: url=%q err=%v", presigned, err)
	}
}

func TestDeclarativeAlertRulesAndChannels(t *testing.T) {
	tenant := &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: "admin"},
		Spec: corev1.TenantSpec{
			AlertRules: []corev1.AlertRule{
				{Title: "Custom Memory Rule", Metric: corev1.AlertMetricMemoryUsageRatio, Threshold: 92, Enabled: true},
			},
			AlertChannels: []corev1.AlertChannel{
				{URL: "https://open.feishu.cn/open-apis/bot/v2/hook/ws-1", Enabled: true},
			},
			PlatformAlertChannels: []corev1.AlertChannel{
				{URL: "https://oapi.dingtalk.com/robot/send?access_token=plat-1", Enabled: true},
			},
		},
	}
	rules := alertmodel.ExtractTenantRules(tenant)
	if len(rules) != 1 || rules[0].ID != "rule-1" || rules[0].Threshold != 92 || rules[0].ForSeconds != 120 || rules[0].Severity != corev1.AlertSeverityWarning {
		t.Fatalf("unexpected extracted rules: %+v", rules)
	}
	wsCh := alertmodel.ExtractTenantChannels(tenant)
	if len(wsCh) != 1 || wsCh[0].ID != "ch-1" || wsCh[0].Type != corev1.ChannelTypeFeishu {
		t.Fatalf("unexpected extracted workspace channels: %+v", wsCh)
	}
	platCh := alertmodel.ExtractPlatformChannels(tenant)
	if len(platCh) != 1 || platCh[0].ID != "pch-1" || platCh[0].Type != corev1.ChannelTypeDingTalk {
		t.Fatalf("unexpected extracted platform channels: %+v", platCh)
	}
}

func TestNodeClockSyncEvaluation(t *testing.T) {
	now := time.Now().UTC()
	node := &corev1.Node{
		Metadata: metav1.ObjectMeta{Name: "node-1", Generation: 3},
		Status:   corev1.NodeStatus{LastHeartbeatTime: &now},
	}
	nodemodel.EvaluateNodeClockSync(node, node.Metadata.Generation)
	if len(node.Status.Conditions) != 1 ||
		node.Status.Conditions[0].Type != metav1.ConditionTypeClockSynced ||
		node.Status.Conditions[0].Status != metav1.ConditionTrue ||
		node.Status.Conditions[0].Reason != corev1.ReasonClockSynced {
		t.Fatalf("expected ClockSynced=True, got %+v", node.Status.Conditions)
	}

	skewed := now.Add(-2 * time.Minute)
	node.Status.LastHeartbeatTime = &skewed
	nodemodel.EvaluateNodeClockSync(node, node.Metadata.Generation)
	if len(node.Status.Conditions) != 1 ||
		node.Status.Conditions[0].Type != metav1.ConditionTypeClockSynced ||
		node.Status.Conditions[0].Status != metav1.ConditionFalse ||
		node.Status.Conditions[0].Reason != corev1.ReasonClockSkewDetected {
		t.Fatalf("expected ClockSynced=False (ClockSkewDetected), got %+v", node.Status.Conditions)
	}
}

func TestAgentPerInstancePrometheusMetricsDefault(t *testing.T) {
	if !config.New().Agent.PerInstancePrometheusMetrics {
		t.Fatal("expected config.New().Agent.PerInstancePrometheusMetrics to default to true")
	}
	if !config.NewAgentConfig().PerInstancePrometheusMetrics {
		t.Fatal("expected config.NewAgentConfig().PerInstancePrometheusMetrics to default to true")
	}
	loadedYAML, err := config.LoadFile("../configs/node-agent.yaml")
	if err != nil {
		t.Fatalf("LoadFile(node-agent.yaml): %v", err)
	}
	if !loadedYAML.Agent.PerInstancePrometheusMetrics {
		t.Fatal("expected node-agent.yaml without explicit override to keep PerInstancePrometheusMetrics=true")
	}

	t.Setenv("GP_AGENT_PER_INSTANCE_PROMETHEUS_METRICS", "false")
	envDisabled, err := config.Load(nil)
	if err != nil {
		t.Fatalf("config.Load(nil) with env override: %v", err)
	}
	if envDisabled.Agent.PerInstancePrometheusMetrics {
		t.Fatal("expected GP_AGENT_PER_INSTANCE_PROMETHEUS_METRICS=false to disable per-instance metrics")
	}

	t.Setenv("GP_AGENT_PER_INSTANCE_PROMETHEUS_METRICS", "true")
	flagDisabled, err := config.Load([]string{"--agent-per-instance-prometheus-metrics=false"})
	if err != nil {
		t.Fatalf("config.Load(flag=false): %v", err)
	}
	if flagDisabled.Agent.PerInstancePrometheusMetrics {
		t.Fatal("expected --agent-per-instance-prometheus-metrics=false flag to override default")
	}
}
