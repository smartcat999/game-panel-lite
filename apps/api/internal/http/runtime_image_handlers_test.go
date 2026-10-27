package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestRuntimeInstallStateReportsUpdateWhenOlderVersionIsInstalled(t *testing.T) {
	router, _, cfg := newTestRouterWithAdapterAndInstallMarkers(t, availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}, false)
	const (
		providerKey      = "palworld"
		installedVersion = "v2.5.0"
		targetVersion    = "v2.7.1"
	)
	archiveRelPath := filepath.Join("runtime-images", providerKey, installedVersion+".tar")
	archivePath := filepath.Join(cfg.DataDir, archiveRelPath)
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, []byte("palworld runtime archive\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(cfg.DataDir, "runtime-installs", providerKey, installedVersion+".json")
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := runtimeInstallMarker{
		ProviderKey: domain.ProviderPalworld,
		Version:     installedVersion,
		Image:       "smartcat99999/palworld-server:" + installedVersion,
		ArchivePath: archiveRelPath,
		InstalledAt: time.Now(),
	}
	data, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, data, 0o644); err != nil {
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
		if game.Key != domain.GamePalworld {
			continue
		}
		for _, provider := range game.Providers {
			if provider.Key != domain.ProviderPalworld {
				continue
			}
			status := provider.RuntimeImage
			if status.Status != runtime.ImageStatusUpdateReady {
				t.Fatalf("expected update available, got %+v", status)
			}
			if status.InstalledVersion != installedVersion || status.TargetVersion != targetVersion {
				t.Fatalf("expected %s -> %s, got %+v", installedVersion, targetVersion, status)
			}
			return
		}
	}
	t.Fatal("expected Palworld provider in catalog")
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

type measuredRuntimeAdapter struct {
	availableMockAdapter
	saving   chan struct{}
	release  chan struct{}
	failSave bool
}

func (a *measuredRuntimeAdapter) PrepareImageWithProgress(_ context.Context, _ string, report runtime.ImagePrepareProgressFunc) error {
	report(runtime.ImagePrepareProgress{Phase: "downloading", CurrentBytes: 100})
	report(runtime.ImagePrepareProgress{Phase: "extracting", CompletedLayers: 2})
	return nil
}
func (a *measuredRuntimeAdapter) SaveImageArchiveWithProgress(ctx context.Context, image, path string, report runtime.ImagePrepareProgressFunc) error {
	report(runtime.ImagePrepareProgress{Phase: "saving", CurrentBytes: 128})
	close(a.saving)
	select {
	case <-a.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	if a.failSave {
		return errors.New("archive write failed")
	}
	return a.SaveImageArchive(ctx, image, path)
}

func TestRuntimeInstallReportsSavingBytesBeforeSuccessOrFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			adapter := &measuredRuntimeAdapter{availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}, saving: make(chan struct{}), release: make(chan struct{}), failSave: fail}
			router, _, _ := newTestRouterWithAdapterAndInstallMarkers(t, adapter, false)
			req := httptest.NewRecorder()
			router.ServeHTTP(req, httptest.NewRequest(stdhttp.MethodPost, "/api/runtime/images/prepare", strings.NewReader(`{"providerKey":"terraria-vanilla"}`)))
			if req.Code != stdhttp.StatusAccepted {
				t.Fatalf("prepare: %d %s", req.Code, req.Body.String())
			}
			select {
			case <-adapter.saving:
			case <-time.After(3 * time.Second):
				t.Fatal("save phase not reached")
			}
			readStatus := func() domain.RuntimeImageStatus {
				t.Helper()
				out := httptest.NewRecorder()
				router.ServeHTTP(out, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
				var games []domain.GameCatalogEntry
				if err := json.Unmarshal(out.Body.Bytes(), &games); err != nil {
					t.Fatal(err)
				}
				for _, g := range games {
					for _, p := range g.Providers {
						if p.Key == domain.ProviderTerrariaVanilla {
							return p.RuntimeImage
						}
					}
				}
				t.Fatal("provider absent")
				return domain.RuntimeImageStatus{}
			}
			got := readStatus()
			close(adapter.release)
			if got.Status != "preparing" || got.Phase != "saving" || got.CurrentBytes != 128 || got.Progress != 0 || got.TotalBytes != 0 {
				t.Fatalf("unmeasured cache should not claim 95%%: %+v", got)
			}
			want := "ready"
			if fail {
				want = "failed"
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				got = readStatus()
				if got.Status == want {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if got.Status != want {
				t.Fatalf("final state: %+v", got)
			}
			if fail && (got.Phase != "saving" || got.Progress == 100 || got.CurrentBytes != 128) {
				t.Fatalf("failure falsely completed: %+v", got)
			}
		})
	}
}
