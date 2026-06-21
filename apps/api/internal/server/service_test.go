package server

import (
	"context"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type memoryStore struct {
	items map[string]domain.GameServer
}

func newMemoryStore() *memoryStore {
	return &memoryStore{items: map[string]domain.GameServer{}}
}

func (s *memoryStore) CreateGameServer(_ context.Context, server *domain.GameServer) error {
	s.items[server.ID] = *server
	return nil
}

func (s *memoryStore) SaveGameServer(_ context.Context, server *domain.GameServer) error {
	s.items[server.ID] = *server
	return nil
}

func (s *memoryStore) GetGameServer(_ context.Context, id string) (domain.GameServer, error) {
	return s.items[id], nil
}

func TestServiceCreateInitializesSpecAndStatus(t *testing.T) {
	store := newMemoryStore()
	service := NewService(store)
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	server, err := service.Create(context.Background(), CreateCommand{
		Name:        " Friends ",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	if server.Name != "Friends" {
		t.Fatalf("expected trimmed server name, got %q", server.Name)
	}
	if server.Spec.Generation != 1 {
		t.Fatalf("expected initial generation 1, got %d", server.Spec.Generation)
	}
	if server.Spec.DesiredState != domain.DesiredStopped {
		t.Fatalf("expected desired stopped, got %q", server.Spec.DesiredState)
	}
	if server.Status.Phase != domain.PhasePending {
		t.Fatalf("expected pending phase, got %q", server.Status.Phase)
	}
	if server.Status.ActualState != domain.ActualUnknown {
		t.Fatalf("expected unknown actual state, got %q", server.Status.ActualState)
	}
}

func TestServiceRequestsBumpGeneration(t *testing.T) {
	store := newMemoryStore()
	service := NewService(store)
	server, err := service.Create(context.Background(), CreateCommand{Name: "Friends"})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	started, err := service.RequestStart(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("request start: %v", err)
	}
	if started.Spec.DesiredState != domain.DesiredRunning {
		t.Fatalf("expected desired running, got %q", started.Spec.DesiredState)
	}
	if started.Spec.Generation != 2 {
		t.Fatalf("expected generation 2, got %d", started.Spec.Generation)
	}

	deleting, err := service.RequestDelete(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("request delete: %v", err)
	}
	if deleting.Spec.DesiredState != domain.DesiredDeleted {
		t.Fatalf("expected desired deleted, got %q", deleting.Spec.DesiredState)
	}
	if deleting.Status.Phase != domain.PhaseDeleting {
		t.Fatalf("expected deleting phase, got %q", deleting.Status.Phase)
	}
	if deleting.Spec.Generation != 3 {
		t.Fatalf("expected generation 3, got %d", deleting.Spec.Generation)
	}
}
