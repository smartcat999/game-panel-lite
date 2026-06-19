package exporter

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestCollectorExposesLowCardinalityBusinessMetrics(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "gamepanel.db"))
	if err != nil {
		t.Fatal(err)
	}
	server := domain.GameServerInstance{
		ID:          "server-1",
		Name:        "Do Not Expose This Name",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Status:      domain.StatusRunning,
		MaxPlayers:  8,
		Version:     "latest",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateActivity(context.Background(), &domain.ActivityEvent{ID: "event-1", InstanceID: server.ID, Type: "server.started", Message: "Started", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateBackup(context.Background(), &domain.Backup{ID: "backup-1", InstanceID: server.ID, FileName: "backup.zip", SizeBytes: 2048, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateWorld(context.Background(), &domain.World{ID: "world-1", InstanceID: server.ID, Name: "World Name", FileName: "world.wld", SizeBytes: 4096, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateMod(context.Background(), &domain.ModFile{ID: "mod-1", InstanceID: server.ID, FileName: "mod.tmod", SizeBytes: 1024, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	body, err := NewCollector(db).Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"gamepanel_servers_total 1",
		`gamepanel_server_running{game_key="terraria",provider_key="terraria-vanilla",server_id="server-1",status="running",version="latest"} 1`,
		`gamepanel_server_status{game_key="terraria",provider_key="terraria-vanilla",server_id="server-1",status="running"} 1`,
		`gamepanel_backups_total{game_key="terraria",provider_key="terraria-vanilla",server_id="server-1"} 1`,
		`gamepanel_worlds_total{game_key="terraria",provider_key="terraria-vanilla",server_id="server-1"} 1`,
		`gamepanel_mods_total{game_key="terraria",provider_key="terraria-vanilla",server_id="server-1"} 1`,
		`gamepanel_asset_storage_bytes{game_key="terraria",kind="backup",provider_key="terraria-vanilla",server_id="server-1"} 2048`,
		`gamepanel_asset_storage_total_bytes{kind="mod"} 1024`,
		`gamepanel_activity_events_total{severity="success",type="server.started"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in metrics:\n%s", expected, body)
		}
	}
	if strings.Contains(body, server.Name) {
		t.Fatalf("server name should not be exposed as a label:\n%s", body)
	}
	if strings.Contains(body, "backup.zip") || strings.Contains(body, "world.wld") || strings.Contains(body, "mod.tmod") {
		t.Fatalf("asset file names should not be exposed as labels:\n%s", body)
	}
}
