package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type archiveForbiddenAdapter struct {
	availableMockAdapter
	present           atomic.Bool
	exportCalled      atomic.Bool
	pullCalled        atomic.Bool
	offlineLoadCalled atomic.Bool
	verifyFails       bool
	prepareErr        error
}

func (a *archiveForbiddenAdapter) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	status := runtime.ImageStatusMissing
	if a.present.Load() {
		status = runtime.ImageStatusReady
	}
	return domain.RuntimeImageStatus{Image: image, Status: status}
}
func (a *archiveForbiddenAdapter) PrepareImageWithProgress(_ context.Context, _ string, report runtime.ImagePrepareProgressFunc) error {
	a.pullCalled.Store(true)
	if a.prepareErr != nil {
		return a.prepareErr
	}
	if !a.verifyFails {
		a.present.Store(true)
	}
	report(runtime.ImagePrepareProgress{Phase: "finalizing"})
	return nil
}
func (a *archiveForbiddenAdapter) SaveImageArchiveWithProgress(context.Context, string, string, runtime.ImagePrepareProgressFunc) error {
	a.exportCalled.Store(true)
	return fmt.Errorf("write /private/host/path.tar.tmp: no space left on device")
}

func (a *archiveForbiddenAdapter) LoadImageArchive(context.Context, string) error {
	a.offlineLoadCalled.Store(true)
	a.present.Store(true)
	return nil
}

func TestRuntimeInstallDoesNotRequireAnOfflineArchive(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint("cached=", existing), func(t *testing.T) {
			a := &archiveForbiddenAdapter{availableMockAdapter: availableMockAdapter{runtime.NewMockAdapter()}}
			a.present.Store(existing)
			router, _, cfg := newTestRouterWithAdapterAndInstallMarkers(t, a, false)
			out := httptest.NewRecorder()
			router.ServeHTTP(out, httptest.NewRequest(http.MethodPost, "/api/runtime/images/prepare", strings.NewReader(`{"providerKey":"seven-days-to-die"}`)))
			if out.Code != http.StatusAccepted {
				t.Fatalf("prepare: %d %s", out.Code, out.Body.String())
			}
			marker := filepath.Join(cfg.DataDir, "runtime-installs", "seven-days-to-die", "v3.3.0.json")
			var data []byte
			for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
				data, _ = os.ReadFile(marker)
				if len(data) > 0 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if len(data) == 0 {
				t.Fatal("valid Docker image must install even if exporting would run out of disk")
			}
			var record map[string]any
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			if record["storage"] != "docker" || record["archivePath"] != nil {
				t.Fatalf("unexpected mandatory archive: %s", data)
			}
			if existing && a.pullCalled.Load() {
				t.Fatal("cached image was pulled again")
			}
			if a.exportCalled.Load() {
				t.Fatal("installation must not export an image")
			}
			if _, err := os.Stat(filepath.Join(cfg.DataDir, "runtime-images", "seven-days-to-die")); !os.IsNotExist(err) {
				t.Fatalf("archive directory was created: %v", err)
			}
		})
	}
}

func TestRuntimeInstallVerifiesImageAfterPull(t *testing.T) {
	a := &archiveForbiddenAdapter{availableMockAdapter: availableMockAdapter{runtime.NewMockAdapter()}, verifyFails: true}
	router, _, cfg := newTestRouterWithAdapterAndInstallMarkers(t, a, false)
	out := httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequest(http.MethodPost, "/api/runtime/images/prepare", strings.NewReader(`{"providerKey":"seven-days-to-die"}`)))
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		out = httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/games", nil))
		var games []domain.GameCatalogEntry
		if err := json.Unmarshal(out.Body.Bytes(), &games); err != nil {
			t.Fatal(err)
		}
		for _, g := range games {
			for _, p := range g.Providers {
				if p.Key == domain.ProviderSevenDaysToDie && p.RuntimeImage.Status == runtime.ImageStatusFailed {
					if _, err := os.Stat(filepath.Join(cfg.DataDir, "runtime-installs", "seven-days-to-die", "v3.3.0.json")); !os.IsNotExist(err) {
						t.Fatal("unverified image was registered")
					}
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("pull success without an image must fail validation")
}

func TestRuntimeInstallReusesLegacyOfflineArchiveAndCleansOnlyItsTempFile(t *testing.T) {
	a := &archiveForbiddenAdapter{availableMockAdapter: availableMockAdapter{runtime.NewMockAdapter()}}
	router, _, cfg := newTestRouterWithAdapterAndInstallMarkers(t, a, false)
	base := filepath.Join(cfg.DataDir, "runtime-images", "seven-days-to-die")
	archive := filepath.Join(base, "v3.3.0.tar")
	marker := filepath.Join(cfg.DataDir, "runtime-installs", "seven-days-to-die", "v3.3.0.json")
	world := filepath.Join(cfg.DataDir, "instances", "existing", "userdata", "world.save")
	files := map[string]string{archive: "offline archive", archive + ".tmp": "abandoned cache", world: "existing day 49", marker: `{"providerKey":"seven-days-to-die","version":"v3.3.0","image":"smartcat99999/seven-days-to-die:v3.3.0","archivePath":"runtime-images/seven-days-to-die/v3.3.0.tar"}`}
	for path, data := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	out := httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequest(http.MethodPost, "/api/runtime/images/prepare", strings.NewReader(`{"providerKey":"seven-days-to-die"}`)))
	if out.Code != http.StatusAccepted {
		t.Fatalf("prepare: %d", out.Code)
	}
	migrated := false
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		data, _ := os.ReadFile(marker)
		var m runtimeInstallMarker
		if json.Unmarshal(data, &m) == nil && m.Storage == "docker" {
			migrated = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !migrated || !a.offlineLoadCalled.Load() || a.pullCalled.Load() || a.exportCalled.Load() {
		t.Fatal("legacy archive was not reused without download/export")
	}
	if _, err := os.Stat(archive + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("abandoned temp remains")
	}
	for _, path := range []string{archive, world} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != files[path] {
			t.Fatalf("existing file changed: %s", path)
		}
	}
}

func TestRuntimeInstallDiskFailureDoesNotExposeHostPaths(t *testing.T) {
	a := &archiveForbiddenAdapter{availableMockAdapter: availableMockAdapter{runtime.NewMockAdapter()}, prepareErr: fmt.Errorf("write /private/host/image: no space left on device")}
	router, _, _ := newTestRouterWithAdapterAndInstallMarkers(t, a, false)
	out := httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequest(http.MethodPost, "/api/runtime/images/prepare", strings.NewReader(`{"providerKey":"seven-days-to-die"}`)))
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		out = httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/games", nil))
		var games []domain.GameCatalogEntry
		if err := json.Unmarshal(out.Body.Bytes(), &games); err != nil {
			t.Fatal(err)
		}
		for _, g := range games {
			for _, p := range g.Providers {
				if p.Key == domain.ProviderSevenDaysToDie && p.RuntimeImage.Status == runtime.ImageStatusFailed {
					if strings.Contains(p.RuntimeImage.Message, "/private/") || !strings.Contains(p.RuntimeImage.Message, "disk space is insufficient") {
						t.Fatalf("unsafe disk error: %s", p.RuntimeImage.Message)
					}
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("disk error was not reported")
}

func TestRestoreCanRecoverRegisteredLatestImageAfterDockerGC(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapterAndInstallMarkers(t, missingImageAdapter{availableMockAdapter{runtime.NewMockAdapter()}}, true)
	server := domain.GameServer{ID: "recover-registered", Name: "Existing", ProviderKey: domain.ProviderTerrariaTModLoader,
		Spec:   domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped, Version: "v2026.02.3.1", Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(cfg.DataDir, "instances", "recover-registered")}, Config: map[string]any{"serverName": "Existing", "worldName": "Existing", "worldSize": "medium", "difficulty": "classic", "maxPlayers": float64(8), "port": float64(7777), "language": "en-US", "autoCreateWorld": true}},
		Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped, ActualState: domain.ActualStopped},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	out := httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequest(http.MethodPost, "/api/servers/recover-registered/start", nil))
	if out.Code != http.StatusAccepted {
		t.Fatalf("registered latest image must recover: %d %s", out.Code, out.Body.String())
	}
	var updated domain.GameServer
	if err := json.Unmarshal(out.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != server.ID || updated.Spec.Version != "v2026.04.3.0" || updated.Spec.Runtime.DataDir != server.Spec.Runtime.DataDir {
		t.Fatal("original instance was not retained")
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "runtime-images", "terraria-tmodloader", "v2026.02.3.1.tar")); err != nil {
		t.Fatal("old recovery cache removed before new image was available")
	}
}
