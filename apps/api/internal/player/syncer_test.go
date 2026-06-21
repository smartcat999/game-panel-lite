package player

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type playerRuntime struct {
	runtime.MockAdapter
	logs string
}

func (r *playerRuntime) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: true, Message: "ok", Host: "mock"}
}

func (r *playerRuntime) LogSnapshotWorkload(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(r.logs)), nil
}

func TestRunOnceUpdatesRunningServerPlayerCount(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/gamepanel.db")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	server := domain.GameServer{
		ID:          "server-1",
		Name:        "Friends",
		GameKey:     "terraria",
		ProviderKey: domain.ProviderTerrariaTModLoader,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredRunning,
			Version:      "v2026.04.3.0",
			Config: map[string]any{
				"serverName": "Friends",
				"worldName":  "Friends",
				"maxPlayers": 8,
				"port":       terraria.DefaultInternalPort,
			},
			Network: domain.ServerNetworkSpec{Port: terraria.DefaultInternalPort},
			Runtime: domain.ServerRuntimeSpec{DataDir: t.TempDir()},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhaseRunning,
			ActualState:        domain.ActualRunning,
			RuntimeID:          "container-1",
			ObservedGeneration: 1,
			AppliedGeneration:  1,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	runtimeAdapter := &playerRuntime{logs: "Server started\n: yyds (192.168.215.1:32643)\n\n1个玩家已连接。\n"}
	syncer := NewSyncer(
		db,
		provider.NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider()),
		runtimeAdapter,
		config.Config{},
	)

	if err := syncer.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	updated, err := db.GetGameServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status.PlayersOnline != 1 {
		t.Fatalf("expected player count to update to 1, got %+v", updated)
	}
}

func TestRunOnceClearsPlayerCountForStoppedServer(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/gamepanel.db")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	server := domain.GameServer{
		ID:          "server-1",
		Name:        "Friends",
		GameKey:     "terraria",
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredStopped,
			Version:      "1.4.5.6",
			Config: map[string]any{
				"serverName": "Friends",
				"worldName":  "Friends",
				"maxPlayers": 8,
				"port":       terraria.DefaultInternalPort,
			},
			Network: domain.ServerNetworkSpec{Port: terraria.DefaultInternalPort},
			Runtime: domain.ServerRuntimeSpec{DataDir: t.TempDir()},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhaseStopped,
			ActualState:        domain.ActualStopped,
			PlayersOnline:      3,
			ObservedGeneration: 1,
			AppliedGeneration:  1,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	syncer := NewSyncer(
		db,
		provider.NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider()),
		&playerRuntime{},
		config.Config{},
	)

	if err := syncer.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	updated, err := db.GetGameServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status.PlayersOnline != 0 {
		t.Fatalf("expected stopped server player count to reset, got %+v", updated)
	}
}
