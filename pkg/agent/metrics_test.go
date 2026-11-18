package agent

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

func TestEvaluateTerrariaBilingualOnlinePlayersMetric(t *testing.T) {
	spec := corev1.AppMetricSpec{
		ID:           "online_players",
		Label:        "在线玩家",
		Unit:         "人",
		Mode:         corev1.MetricModeRegexValue,
		ProbeCommand: "playing",
		Pattern:      `(No|\d+)(?:\s+players?\s+connected|\s*个玩家已连接)`,
		ResetPattern: `No players connected|无玩家连接`,
		IncPattern:   `([^\r\n:]+?)(?:\s+has joined\.|已加入[。\.])`,
		DecPattern:   `([^\r\n:]+?)(?:\s+has left\.|已离开[。\.])`,
	}

	zhZero := EvaluateDeclarativeMetric(spec, []string{": 无玩家连接。"})
	if zhZero.Value != 0 || zhZero.Display != "0" {
		t.Fatalf("expected 0 on zh-Hans empty server, got %+v", zhZero)
	}

	zhJoined := EvaluateDeclarativeMetric(spec, []string{
		": 无玩家连接。",
		": Alice已加入。",
		"Bob (127.0.0.1:51234)",
		": 2个玩家已连接。",
	})
	if zhJoined.Value != 2 || zhJoined.Display != "2" || len(zhJoined.Items) != 1 || zhJoined.Items[0] != "Alice" {
		t.Fatalf("expected 2 players and Alice delta item on zh-Hans, got %+v", zhJoined)
	}

	enJoined := EvaluateDeclarativeMetric(spec, []string{
		"No players connected.",
		"Charlie has joined.",
		"1 player connected.",
	})
	if enJoined.Value != 1 || enJoined.Display != "1" || len(enJoined.Items) != 1 || enJoined.Items[0] != "Charlie" {
		t.Fatalf("expected 1 player and Charlie delta item on en-US, got %+v", enJoined)
	}
}

func TestEvaluateSteamSurvivalGameMetrics(t *testing.T) {
	palSpec := corev1.AppMetricSpec{
		ID:          "online_players",
		Source:      corev1.MetricSourceExec,
		Mode:        corev1.MetricModeRegexCount,
		Pattern:     `^(.+\s+\([0-9A-Za-z_:-]+\))$`,
		ItemPattern: `^(.+\s+\([0-9A-Za-z_:-]+\))$`,
	}
	palEmpty := EvaluateDeclarativeMetric(palSpec, []string{"name,playeruid,steamid"})
	if palEmpty.Value != 0 || len(palEmpty.Items) != 0 {
		t.Fatalf("palworld empty expected 0, got %+v", palEmpty)
	}
	palTwo := EvaluateDeclarativeMetric(palSpec, []string{"name,playeruid,steamid", "Ash (76561198000000001)", "Misty (76561198000000002)"})
	if palTwo.Value != 2 || !reflect.DeepEqual(palTwo.Items, []string{"Ash (76561198000000001)", "Misty (76561198000000002)"}) {
		t.Fatalf("palworld 2 players unexpected: %+v", palTwo)
	}

	dstSpec := corev1.AppMetricSpec{
		ID:          "online_players",
		Mode:        corev1.MetricModeRegexValue,
		Pattern:     `GP_ONLINE=(\d+)`,
		ItemPattern: `GP_PLAYER=([^\r\n]+)`,
	}
	dstAfterLeave := EvaluateDeclarativeMetric(dstSpec, []string{
		"GP_ONLINE=1",
		"GP_PLAYER=Wilson (KU_abc123)",
		"Sim paused",
		"GP_ONLINE=0",
	})
	if dstAfterLeave.Value != 0 || len(dstAfterLeave.Items) != 0 {
		t.Fatalf("dst after leave expected 0, got %+v", dstAfterLeave)
	}

	sdSpec := corev1.AppMetricSpec{
		ID:          "online_players",
		Source:      corev1.MetricSourceExec,
		Mode:        corev1.MetricModeRegexCount,
		Pattern:     `^\d+\.\s*id=(\d+),\s*([^,]+)`,
		ItemPattern: `^\d+\.\s*id=(\d+),\s*([^,]+)`,
	}
	sdOne := EvaluateDeclarativeMetric(sdSpec, []string{
		"0. id=171, RickGrimes, pos=(-12.3, 61.0, 45.2), remote=True",
		"Total of 1 in the game",
	})
	if sdOne.Value != 1 || !reflect.DeepEqual(sdOne.Items, []string{"RickGrimes (171)"}) {
		t.Fatalf("7dtd 1 player unexpected: %+v", sdOne)
	}
}

func TestBuildConsoleCommandArgv(t *testing.T) {
	if got := runtime.BuildConsoleCommandArgv("", "save"); !reflect.DeepEqual(got, []string{runtime.ExecStdinMarker, "save"}) {
		t.Fatalf("empty delivery mismatch: %v", got)
	}
	if got := runtime.BuildConsoleCommandArgv("rcon-cli", "Save"); !reflect.DeepEqual(got, []string{"rcon-cli", "Save"}) {
		t.Fatalf("rcon-cli delivery mismatch: %v", got)
	}
	if got := runtime.BuildConsoleCommandArgv("dst-send-to-console", "c_save()"); !reflect.DeepEqual(got, []string{"dst-send-to-console", "c_save()"}) {
		t.Fatalf("dst-send-to-console delivery mismatch: %v", got)
	}
	if got := runtime.BuildConsoleCommandArgv("7dtd-send-to-console", "saveworld"); !reflect.DeepEqual(got, []string{"7dtd-send-to-console", "saveworld"}) {
		t.Fatalf("7dtd-send-to-console delivery mismatch: %v", got)
	}
}

type probeStubRuntimeDriver struct {
	execCalls [][]string
	execOut   string
}

func (d *probeStubRuntimeDriver) Handler() string { return "docker" }
func (d *probeStubRuntimeDriver) Version() string { return "stub" }
func (d *probeStubRuntimeDriver) Validate(*corev1.Instance, *corev1.RuntimeClass) error {
	return nil
}
func (d *probeStubRuntimeDriver) Reconcile(context.Context, *corev1.Instance, *corev1.RuntimeClass) (*runtime.RuntimeObservation, error) {
	return nil, nil
}
func (d *probeStubRuntimeDriver) Destroy(context.Context, *corev1.Instance, *corev1.RuntimeClass) error {
	return nil
}
func (d *probeStubRuntimeDriver) Logs(context.Context, *corev1.Instance, runtime.LogOptions) (io.ReadCloser, error) {
	return nil, nil
}
func (d *probeStubRuntimeDriver) Exec(_ context.Context, _ *corev1.Instance, req runtime.ExecRequest) (string, error) {
	d.execCalls = append(d.execCalls, append([]string(nil), req.Command...))
	return d.execOut, nil
}

func TestExecuteCustomMetricProbesUsesConsoleDelivery(t *testing.T) {
	drv := &probeStubRuntimeDriver{}
	inst := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			BackupPolicy: &corev1.InstanceBackupPolicy{ConsoleDelivery: "tml-send-to-console"},
			Metrics:      []corev1.AppMetricSpec{{ID: "online_players", ProbeCommand: "playing"}},
		},
	}
	executeCustomMetricProbes(inst, drv, "test/tml")
	wantTML := [][]string{{"tml-send-to-console", "playing"}}
	if !reflect.DeepEqual(drv.execCalls, wantTML) {
		t.Fatalf("expected %v, got %v", wantTML, drv.execCalls)
	}

	drvFallback := &probeStubRuntimeDriver{execOut: "GP_ONLINE=0\n"}
	instFallback := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Metrics: []corev1.AppMetricSpec{
				{ID: "online_players", ProbeCommand: "playing"},
				{ID: "exec_probe", Source: corev1.MetricSourceExec, ProbeCommand: "echo GP_ONLINE=0"},
			},
		},
	}
	got := executeCustomMetricProbes(instFallback, drvFallback, "test/fallback")
	wantFallback := [][]string{{runtime.ExecStdinMarker, "playing"}, {"sh", "-c", "echo GP_ONLINE=0"}}
	if !reflect.DeepEqual(drvFallback.execCalls, wantFallback) {
		t.Fatalf("expected fallback calls %v, got %v", wantFallback, drvFallback.execCalls)
	}
	if !reflect.DeepEqual(got["exec_probe"], []string{"GP_ONLINE=0"}) {
		t.Fatalf("expected exec probe output [GP_ONLINE=0], got %v", got["exec_probe"])
	}
}

func TestComputeNetworkRateDeltaAndIfaceFilter(t *testing.T) {
	now := time.Unix(1700000010, 0)
	if rx, tx := computeNetworkRateDelta(dockerCPUSample{}, false, 1000, 2000, now); rx != 0 || tx != 0 {
		t.Fatalf("expected zero on first sample, got (%d, %d)", rx, tx)
	}
	prev := dockerCPUSample{rxBytes: 1000, txBytes: 2000, sampledAt: now.Add(-5 * time.Second)}
	if rx, tx := computeNetworkRateDelta(prev, true, 11000, 7000, now); rx != 2000 || tx != 1000 {
		t.Fatalf("expected (2000, 1000), got (%d, %d)", rx, tx)
	}
	if rx, tx := computeNetworkRateDelta(prev, true, 500, 1000, now); rx != 0 || tx != 0 {
		t.Fatalf("expected counter reset guard (0, 0), got (%d, %d)", rx, tx)
	}
	for _, virt := range []string{"lo", "veth123", "docker0", "br-abc", "tun0", "tailscale0", "Meta", "edge-tap11"} {
		if !isVirtualOrLoopbackIface(virt) {
			t.Fatalf("expected %q to be filtered as virtual/loopback", virt)
		}
		if isPhysicalHostIface(virt) {
			t.Fatalf("expected %q not to match physical host iface", virt)
		}
	}
	for _, phys := range []string{"eth0", "enp3s0", "wlp2s0", "wlan0"} {
		if isVirtualOrLoopbackIface(phys) {
			t.Fatalf("expected %q to be kept as host physical/primary iface", phys)
		}
		if !isPhysicalHostIface(phys) {
			t.Fatalf("expected %q to match physical host iface", phys)
		}
	}
}

func TestShortIntervalNetworkSamplingPreservesRateAndBaseline(t *testing.T) {
	t0 := time.Unix(1700000000, 0)
	collector := NewHostResourceCollector(t.TempDir())
	if rx, tx := collector.updateNetworkRates(1000, 500, t0); rx != 0 || tx != 0 {
		t.Fatalf("host first sample expected (0,0), got (%d,%d)", rx, tx)
	}
	if rx, tx := collector.updateNetworkRates(5000, 2500, t0.Add(2*time.Second)); rx != 2000 || tx != 1000 {
		t.Fatalf("host 2s sample expected (2000,1000), got (%d,%d)", rx, tx)
	}
	if rx, tx := collector.updateNetworkRates(5100, 2600, t0.Add(2050*time.Millisecond)); rx != 2000 || tx != 1000 {
		t.Fatalf("host <0.2s sample should preserve cached rate (2000,1000), got (%d,%d)", rx, tx)
	}
	if rx, tx := collector.updateNetworkRates(11000, 6500, t0.Add(4*time.Second)); rx != 3000 || tx != 2000 {
		t.Fatalf("host next >=0.2s sample expected (3000,2000) from preserved baseline, got (%d,%d)", rx, tx)
	}

	cid := "test-short-interval-container"
	t.Cleanup(func() {
		dockerCPUMu.Lock()
		delete(dockerCPUMap, cid)
		dockerCPUMu.Unlock()
	})
	makeStats := func(rx, tx uint64) *container.StatsResponse {
		return &container.StatsResponse{
			Networks: map[string]container.NetworkStats{"eth0": {RxBytes: rx, TxBytes: tx}},
		}
	}
	_, rx0, tx0 := computeDockerStatsRatesAt(cid, makeStats(1000, 500), t0)
	if rx0 != 0 || tx0 != 0 {
		t.Fatalf("container first sample expected (0,0), got (%d,%d)", rx0, tx0)
	}
	_, rx1, tx1 := computeDockerStatsRatesAt(cid, makeStats(5000, 2500), t0.Add(2*time.Second))
	if rx1 != 2000 || tx1 != 1000 {
		t.Fatalf("container 2s sample expected (2000,1000), got (%d,%d)", rx1, tx1)
	}
	_, rxShort, txShort := computeDockerStatsRatesAt(cid, makeStats(5100, 2550), t0.Add(2050*time.Millisecond))
	if rxShort != 2000 || txShort != 1000 {
		t.Fatalf("container <0.2s sample should preserve cached rate (2000,1000), got (%d,%d)", rxShort, txShort)
	}
	_, rx2, tx2 := computeDockerStatsRatesAt(cid, makeStats(11000, 6500), t0.Add(4*time.Second))
	if rx2 != 3000 || tx2 != 2000 {
		t.Fatalf("container next >=0.2s sample expected (3000,2000) from preserved baseline, got (%d,%d)", rx2, tx2)
	}
}
