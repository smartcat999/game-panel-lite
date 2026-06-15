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
	commands []string
	logs     string
}

func (r *playerRuntime) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: true, Message: "ok", Host: "mock"}
}

func (r *playerRuntime) SendCommand(_ context.Context, _ domain.GameServerInstance, command string) error {
	r.commands = append(r.commands, command)
	return nil
}

func (r *playerRuntime) Logs(context.Context, domain.GameServerInstance) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(r.logs)), nil
}

func TestRunOnceUpdatesRunningServerPlayerCount(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/gamepanel.db")
	if err != nil {
		t.Fatal(err)
	}
	server := domain.GameServerInstance{
		ID:          "server-1",
		Name:        "Friends",
		GameKey:     "terraria",
		ProviderKey: domain.ProviderTerrariaVanilla,
		Status:      domain.StatusRunning,
		WorldName:   "Friends",
		Port:        terraria.DefaultInternalPort,
		MaxPlayers:  8,
		DataDir:     t.TempDir(),
		ContainerID: "container-1",
		Config:      terraria.Presets[0].Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	runtimeAdapter := &playerRuntime{logs: "Server started\nPlayers connected: Alice, Bob\n"}
	syncer := NewSyncer(
		db,
		provider.NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider()),
		runtimeAdapter,
		config.Config{},
	)
	syncer.ResponseDelay = 0

	if err := syncer.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(runtimeAdapter.commands) != 1 || runtimeAdapter.commands[0] != "playing" {
		t.Fatalf("expected playing command to be sent, got %+v", runtimeAdapter.commands)
	}
	updated, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PlayersOnline != 2 {
		t.Fatalf("expected player count to update to 2, got %+v", updated)
	}
}

func TestRunOnceClearsPlayerCountForStoppedServer(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/gamepanel.db")
	if err != nil {
		t.Fatal(err)
	}
	server := domain.GameServerInstance{
		ID:            "server-1",
		Name:          "Friends",
		GameKey:       "terraria",
		ProviderKey:   domain.ProviderTerrariaVanilla,
		Status:        domain.StatusStopped,
		WorldName:     "Friends",
		Port:          terraria.DefaultInternalPort,
		MaxPlayers:    8,
		PlayersOnline: 3,
		DataDir:       t.TempDir(),
		Config:        terraria.Presets[0].Config,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
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

	updated, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PlayersOnline != 0 {
		t.Fatalf("expected stopped server player count to reset, got %+v", updated)
	}
}
