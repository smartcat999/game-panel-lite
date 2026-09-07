package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestDependencyLookupStaysWithinProvider(t *testing.T) {
	_, db, _ := newTestRouter(t)
	resolver := &Handler{store: db}
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

func TestAssignModRejectsCrossProviderBeforeMutation(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("target", cfg.DataDir)
	server.ProviderKey = domain.ProviderDST
	createTestServer(t, db, server)
	item := domain.ModFile{ID: "foreign", InstanceID: "unassigned", ProviderKey: domain.ProviderTerrariaTModLoader, Source: "workshop", WorkshopID: "123", FileName: "workshop-123"}
	if err := db.CreateMod(context.Background(), &item); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/mods/foreign/assign", strings.NewReader(`{"instanceId":"target"}`)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "mod provider does not match") {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
	mods, err := db.ListMods(context.Background(), "target")
	if err != nil || len(mods) != 0 {
		t.Fatalf("unexpected mutation: %+v %v", mods, err)
	}
}
