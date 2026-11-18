package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestGameServerCRUDPersistsSpecAndStatus(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gamepanel.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	server := domain.GameServer{
		ID:          "srv-1",
		Name:        "Friends",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			Generation:   2,
			DesiredState: domain.DesiredRunning,
			Version:      "1.4.5.6",
			Config:       map[string]any{"worldName": "Friends World"},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhasePending,
			ActualState:        domain.ActualUnknown,
			ObservedGeneration: 1,
		},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatalf("create game server: %v", err)
	}

	stored, err := db.GetGameServer(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("get game server: %v", err)
	}
	if stored.Spec.DesiredState != domain.DesiredRunning {
		t.Fatalf("expected desired running, got %q", stored.Spec.DesiredState)
	}
	if stored.Spec.Config["worldName"] != "Friends World" {
		t.Fatalf("expected config payload to round trip, got %#v", stored.Spec.Config)
	}
	if stored.Status.ObservedGeneration != 1 {
		t.Fatalf("expected observed generation 1, got %d", stored.Status.ObservedGeneration)
	}
}
