package server

import (
	"context"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	"path/filepath"
	"testing"
)

func TestDependencyLookupStaysWithinProvider(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := &RuntimeModPlanner{store: db}
	ctx := context.Background()
	for _, instance := range []string{"unassigned", "server"} {
		for _, key := range []domain.ProviderKey{domain.ProviderPalworld, domain.ProviderTerrariaTModLoader} {
			item := domain.ModFile{ID: instance + string(key), InstanceID: instance, ProviderKey: key, FileName: string(key) + ".tmod", ModName: "SharedDependency"}
			if err := db.CreateMod(ctx, &item); err != nil {
				t.Fatal(err)
			}
		}
	}
	item, ok, err := resolver.findLibraryModByModName(ctx, domain.ProviderTerrariaTModLoader, "SharedDependency")
	if err != nil || !ok || item.ProviderKey != domain.ProviderTerrariaTModLoader {
		t.Fatalf("wrong library dependency: %+v %v %v", item, ok, err)
	}
	item, ok, err = resolver.findServerModByModName(ctx, "server", domain.ProviderTerrariaTModLoader, "SharedDependency")
	if err != nil || !ok || item.ProviderKey != domain.ProviderTerrariaTModLoader {
		t.Fatalf("wrong server dependency: %+v %v %v", item, ok, err)
	}
	if _, ok, err := resolver.findLibraryModByModName(ctx, domain.ProviderDST, "SharedDependency"); err != nil || ok {
		t.Fatalf("cross-game fallback: %v %v", ok, err)
	}
}

func TestPlannerRejectsCrossProviderAssignmentBeforeIO(t *testing.T) {
	planner := &RuntimeModPlanner{}
	_, err := planner.assignLibraryMod(context.Background(), domain.GameServer{ProviderKey: domain.ProviderDST}, domain.ModFile{ProviderKey: domain.ProviderTerrariaTModLoader, InstanceID: "unassigned", Source: "workshop", WorkshopID: "123"})
	if err == nil {
		t.Fatal("cross-provider assignment accepted")
	}
}
