package nodeworkload

import (
	"context"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceobservability"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

func TestRuntimeReplacementPreservesInstanceHistoryAndInternetEgress(t *testing.T) {
	now := time.Now().UTC()
	sink := &MemoryTelemetry{}
	game := &FakeGameProvider{Artifact: "fixture:v1", Metrics: []instanceobservability.MetricSample{{ID: "met_players", Metric: "players.online", Value: 2, Unit: "count", Source: "provider-api", SampledAt: now}}}
	manifest := providercontract.Manifest{ProviderReleaseID: "gpr_fixture", Capabilities: []string{"logs", "game-metrics", "console", "backup"}, Metrics: []providercontract.Metric{{Key: "players.online", Source: "provider-api"}}}
	intent := Intent{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_one", RegionalDeploymentID: "rdp_one", ProviderReleaseID: "gpr_fixture", GameVersion: "1.0", ApplyBehavior: "recreate-required", DataScope: "instances/lin_one"}
	firstRuntime := &FakeRuntimeProvider{Name: "alpha"}
	first := New(game, firstRuntime, sink, []string{"10.0.0.0/8"})
	if _, err := first.Reconcile(context.Background(), manifest, intent, 1, now); err != nil {
		t.Fatal(err)
	}
	secondRuntime := &FakeRuntimeProvider{Name: "beta"}
	second := New(game, secondRuntime, sink, []string{"10.0.0.0/8"})
	if _, err := second.Reconcile(context.Background(), manifest, intent, 2, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	history := sink.ByInstance[intent.LogicalInstanceID]
	if len(history) != 2 || history[0].RuntimeAttemptID == history[1].RuntimeAttemptID {
		t.Fatalf("instance history=%#v", history)
	}
	if !firstRuntime.Policies[0].InternetEgressAllowed || len(firstRuntime.Policies[0].DeniedManagementCIDRs) != 1 {
		t.Fatalf("network policy=%#v", firstRuntime.Policies[0])
	}
}

func TestGameMetricsAreConditional(t *testing.T) {
	now := time.Now().UTC()
	sink := &MemoryTelemetry{}
	game := &FakeGameProvider{Metrics: []instanceobservability.MetricSample{{ID: "met_players", Metric: "players.online", Source: "provider-api", SampledAt: now}}}
	module := New(game, &FakeRuntimeProvider{Name: "one"}, sink, nil)
	intent := Intent{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_one", RegionalDeploymentID: "rdp_one", ProviderReleaseID: "gpr_fixture", GameVersion: "1.0", ApplyBehavior: "recreate-required", DataScope: "instances/lin_one"}
	if _, err := module.Reconcile(context.Background(), providercontract.Manifest{ProviderReleaseID: "gpr_fixture", Capabilities: []string{"logs"}}, intent, 1, now); err != nil {
		t.Fatal(err)
	}
	if got := len(sink.ByInstance["lin_one"][0].Metrics); got != 1 {
		t.Fatalf("platform-only metrics=%d", got)
	}
}

func TestConsoleAndBackupAreCapabilityGated(t *testing.T) {
	game := &FakeGameProvider{}
	runtime := &FakeRuntimeProvider{Name: "one"}
	module := New(game, runtime, &MemoryTelemetry{}, nil)
	manifest := providercontract.Manifest{Capabilities: []string{"console", "backup"}}
	if err := module.Console(context.Background(), manifest, RuntimeHandle{RuntimeAttemptID: "rta_one"}, "status"); err != nil || len(game.ConsoleHistory) != 1 {
		t.Fatalf("console history=%#v err=%v", game.ConsoleHistory, err)
	}
	artifact, err := module.Backup(context.Background(), manifest, "lin_one", "object://backup")
	if err != nil || artifact.ObjectKey == "" {
		t.Fatalf("artifact=%#v err=%v", artifact, err)
	}
	if err := module.Restore(context.Background(), manifest, "lin_one", artifact); err != nil {
		t.Fatal(err)
	}
	if err := module.Console(context.Background(), providercontract.Manifest{}, RuntimeHandle{}, "status"); err != ErrUnsupportedCapability {
		t.Fatalf("unsupported console err=%v", err)
	}
}
