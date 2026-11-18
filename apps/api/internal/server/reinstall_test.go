package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

func TestReinstallRecreatesOriginalServerAndMountsAfterContainerRemoval(t *testing.T) {
	dataDir := t.TempDir()
	files := map[string]string{"Worlds/Existing.wld": "saved world day 49", "Mods/kept.tmod": "existing mod", "ModConfigs/kept.json": "existing mod config"}
	for name, value := range files {
		path := filepath.Join(dataDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	registry := provider.NewRegistry(terraria.NewTModLoaderProvider(runtimecatalog.Catalog{}))
	adapter := &captureWorkloadRuntime{status: domain.WorkloadStatus{State: domain.ActualMissing}}
	images := &fakeImageLoader{}
	reconciler := NewRuntimeReconciler(NewProviderWorkloadBuilder(registry), NewRuntimeAdapterClient(adapter)).WithImageLoader(images)
	server := domain.GameServer{
		ID: "existing", Name: "Existing", ProviderKey: domain.ProviderTerrariaTModLoader,
		Spec: domain.ServerSpec{Generation: 2, DesiredState: domain.DesiredRunning, Version: "v2026.04.3.0",
			Runtime: domain.ServerRuntimeSpec{DataDir: dataDir}, ModIDs: []string{"kept"},
			Config: map[string]any{"serverName": "Existing", "worldName": "Existing", "worldSize": "medium", "difficulty": "classic", "maxPlayers": float64(8), "port": float64(7777), "language": "en-US", "autoCreateWorld": true},
		},
		Status: domain.ServerRuntimeStatus{RuntimeID: "removed-by-uninstall", Phase: domain.PhasePending, AppliedGeneration: 2, ActualState: domain.ActualStopped},
	}
	updated, err := reconciler.Reconcile(context.Background(), server)
	if err != nil || updated.Status.Phase != domain.PhaseRunning {
		t.Fatalf("server failed to resume: %+v, %v", updated.Status, err)
	}
	if updated.ID != server.ID || updated.Spec.Version != server.Spec.Version || updated.Spec.Runtime.DataDir != dataDir || len(updated.Spec.ModIDs) != 1 {
		t.Fatal("original instance metadata changed")
	}
	if adapter.spec.ServerID != server.ID || adapter.spec.DataDir != dataDir || !strings.HasSuffix(adapter.spec.Image, ":"+server.Spec.Version) {
		t.Fatalf("wrong replacement workload: %+v", adapter.spec)
	}
	if adapter.startID == "" || images.calls != 1 {
		t.Fatal("original image must be loaded before starting replacement")
	}
	if !strings.Contains(strings.Join(adapter.spec.Options.DataMounts, " "), "Mods:/home/container/Mods") {
		t.Fatal("original mods must be remounted")
	}
	for name, value := range files {
		got, err := os.ReadFile(filepath.Join(dataDir, name))
		if err != nil || string(got) != value {
			t.Fatalf("persistent file changed: %s, %v", name, err)
		}
	}
}
