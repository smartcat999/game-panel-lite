package http

import (
	"bytes"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

func TestRuntimeInstallStateDoesNotBlockServerCreation(t *testing.T) {
	router, _, cfg := newTestRouterWithAdapterAndInstallMarkers(t, availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}, false)

	runtimeStatus := func() domain.RuntimeImageStatus {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
		if recorder.Code != stdhttp.StatusOK {
			t.Fatalf("expected game catalog 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var games []domain.GameCatalogEntry
		if err := json.Unmarshal(recorder.Body.Bytes(), &games); err != nil {
			t.Fatal(err)
		}
		for _, game := range games {
			if game.Key != domain.GameTerraria {
				continue
			}
			for _, providerCatalog := range game.Providers {
				if providerCatalog.Key == domain.ProviderTerrariaVanilla {
					return providerCatalog.RuntimeImage
				}
			}
		}
		t.Fatal("expected Terraria vanilla provider in catalog")
		return domain.RuntimeImageStatus{}
	}

	if status := runtimeStatus(); status.Status != runtime.ImageStatusMissing {
		t.Fatalf("expected missing without local install marker and archive, got %+v", status)
	}

	createPayload := `{
		"name":"Vanilla Test",
		"providerKey":"terraria-vanilla",
		"version":"1.4.5.6",
		"hostPort":17777,
		"config":{
			"serverName":"Vanilla Test",
			"worldName":"TestWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":8,
			"port":7777,
			"secure":true,
			"language":"zh-Hans",
			"autoCreateWorld":true
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(createPayload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server to accept configuration without local runtime install, got %d: %s", create.Code, create.Body.String())
	}

	markerPath := filepath.Join(cfg.DataDir, "runtime-installs", "terraria-vanilla", "1.4.5.6.json")
	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected runtime install marker not to be repaired from Docker cache, got %v", err)
	}
}

func TestRuntimeInstallPullProgressIsReservedBelowComplete(t *testing.T) {
	tests := []struct {
		name     string
		progress int
		want     int
	}{
		{name: "empty", progress: 0, want: 0},
		{name: "half", progress: 50, want: 45},
		{name: "nearly done", progress: 99, want: 89},
		{name: "pull complete", progress: 100, want: 90},
		{name: "over complete", progress: 120, want: 90},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeInstallPullProgress(test.progress); got != test.want {
				t.Fatalf("expected %d, got %d", test.want, got)
			}
		})
	}
}

func TestRuntimeInstallStateUsesLocalArchiveWhenDockerImageIsMissing(t *testing.T) {
	router, _, _ := newTestRouterWithAdapter(t, missingImageAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected game catalog 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, game := range games {
		if game.Key != domain.GameTerraria {
			continue
		}
		for _, providerCatalog := range game.Providers {
			if providerCatalog.Key != domain.ProviderTerrariaVanilla {
				continue
			}
			found = true
			if providerCatalog.RuntimeImage.Status != runtime.ImageStatusReady {
				t.Fatalf("expected local marker and archive to mark runtime ready, got %+v", providerCatalog.RuntimeImage)
			}
		}
	}
	if !found {
		t.Fatal("expected Terraria vanilla provider in catalog")
	}
}

func TestRuntimeInstallStateDoesNotTrustMarkerWhenArchiveIsMissing(t *testing.T) {
	router, _, cfg := newTestRouterWithAdapterAndInstallMarkers(t, availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}, true)
	archivePath := filepath.Join(cfg.DataDir, "runtime-images", "terraria-vanilla", "1.4.5.6.tar")
	if err := os.Remove(archivePath); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected game catalog 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	for _, game := range games {
		if game.Key != domain.GameTerraria {
			continue
		}
		for _, providerCatalog := range game.Providers {
			if providerCatalog.Key != domain.ProviderTerrariaVanilla {
				continue
			}
			if providerCatalog.RuntimeImage.Status != runtime.ImageStatusMissing {
				t.Fatalf("expected missing archive to make runtime missing, got %+v", providerCatalog.RuntimeImage)
			}
			return
		}
	}
	t.Fatal("expected Terraria vanilla provider in catalog")
}

func TestRuntimeInstallStateDoesNotTrustMarkerWhenArchiveIsEmpty(t *testing.T) {
	router, _, cfg := newTestRouterWithAdapterAndInstallMarkers(t, availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}, true)
	archivePath := filepath.Join(cfg.DataDir, "runtime-images", "terraria-vanilla", "1.4.5.6.tar")
	if err := os.WriteFile(archivePath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected game catalog 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	for _, game := range games {
		if game.Key != domain.GameTerraria {
			continue
		}
		for _, providerCatalog := range game.Providers {
			if providerCatalog.Key != domain.ProviderTerrariaVanilla {
				continue
			}
			if providerCatalog.RuntimeImage.Status != runtime.ImageStatusMissing {
				t.Fatalf("expected empty archive to make runtime missing, got %+v", providerCatalog.RuntimeImage)
			}
			if providerCatalog.RuntimeImage.Message != "runtime image archive is empty" {
				t.Fatalf("expected empty archive message, got %+v", providerCatalog.RuntimeImage)
			}
			return
		}
	}
	t.Fatal("expected Terraria vanilla provider in catalog")
}

func TestGameCatalogMarksDSTUnsupportedOnArmRuntime(t *testing.T) {
	router, _, _ := newTestRouterWithAdapter(t, armMockAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected game catalog 200, got %d: %s", list.Code, list.Body.String())
	}
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(list.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	var dstGame *domain.GameCatalogEntry
	for index := range games {
		if games[index].Key == domain.GameDST {
			dstGame = &games[index]
			break
		}
	}
	if dstGame == nil || dstGame.Status != "unsupported" {
		t.Fatalf("expected DST to be marked unsupported on arm runtime, got %+v", dstGame)
	}

	detail := httptest.NewRecorder()
	router.ServeHTTP(detail, httptest.NewRequest(stdhttp.MethodGet, "/api/games/dont-starve-together", nil))
	if detail.Code != stdhttp.StatusOK {
		t.Fatalf("expected DST game detail 200, got %d: %s", detail.Code, detail.Body.String())
	}
	var got domain.GameCatalogEntry
	if err := json.Unmarshal(detail.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "unsupported" {
		t.Fatalf("expected DST game detail to be unsupported on arm runtime, got %+v", got)
	}
}
