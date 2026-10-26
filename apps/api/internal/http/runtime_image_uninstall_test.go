package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type uninstallTestAdapter struct {
	availableMockAdapter
	removed []string
	stopped []runtime.ImageRemovalTarget
	err     error
}

func (a *uninstallTestAdapter) RemoveImage(_ context.Context, image string, stopped []runtime.ImageRemovalTarget) error {
	a.removed = append(a.removed, image)
	a.stopped = append(a.stopped, stopped...)
	return a.err
}

func TestUninstallRuntimeImagePreservesServerData(t *testing.T) {
	for _, tc := range []struct {
		name        string
		providerKey domain.ProviderKey
		referenced  bool
		running     bool
		runtimeErr  error
		wantStatus  int
	}{
		{name: "uninstall", providerKey: domain.ProviderTerrariaVanilla, wantStatus: http.StatusOK},
		{name: "stopped instance keeps data and can uninstall", providerKey: domain.ProviderTerrariaVanilla, referenced: true, wantStatus: http.StatusOK},
		{name: "running instance blocks removal", providerKey: domain.ProviderTerrariaVanilla, referenced: true, running: true, wantStatus: http.StatusConflict},
		{name: "container reference keeps archive", providerKey: domain.ProviderTerrariaVanilla, runtimeErr: errors.New("runtime image is used by a container"), wantStatus: http.StatusConflict},
		{name: "unknown provider cannot escape cache directory", providerKey: "../../other", wantStatus: http.StatusNotFound},
		{name: "provider required", wantStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &uninstallTestAdapter{availableMockAdapter: availableMockAdapter{runtime.NewMockAdapter()}, err: tc.runtimeErr}
			router, db, cfg := newTestRouterWithAdapterAndInstallMarkers(t, adapter, false)
			archive := filepath.Join(cfg.DataDir, "runtime-images", "terraria-vanilla", "1.4.5.6.tar")
			marker := filepath.Join(cfg.DataDir, "runtime-installs", "terraria-vanilla", "1.4.5.6.json")
			world := filepath.Join(cfg.DataDir, "instances", "saved", "Worlds", "saved.wld")
			config := filepath.Join(cfg.DataDir, "instances", "saved", "serverconfig.txt")
			mod := filepath.Join(cfg.DataDir, "instances", "saved", "Mods", "existing.tmod")
			installed := filepath.Join(cfg.DataDir, "instances", "saved", ".gamepanel-installed")
			backup := filepath.Join(cfg.DataDir, "backups", "saved", "backup.zip")
			files := map[string][]byte{
				archive: []byte("image archive"), archive + ".tmp": []byte("partial archive"),
				marker: []byte(`{"providerKey":"terraria-vanilla","version":"1.4.5.6","image":"smartcat99999/terraria-vanilla:1.4.5.6"}`),
				world:  []byte("saved world"), backup: []byte("saved backup"),
				config: []byte("saved configuration"), mod: []byte("saved mod"), installed: []byte("existing install"),
			}
			for path, contents := range files {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, contents, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.referenced {
				server := domain.GameServer{ID: "saved", Name: "Existing world", ProviderKey: domain.ProviderTerrariaVanilla,
					Spec:   domain.ServerSpec{Version: "1.4.5.6", DesiredState: domain.DesiredStopped, ModIDs: []string{"existing-mod"}, Config: map[string]any{"worldName": "Existing world"}},
					Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped, ActualState: domain.ActualStopped},
				}
				if tc.running {
					server.Spec.DesiredState = domain.DesiredRunning
					server.Status.Phase = domain.PhaseRunning
					server.Status.ActualState = domain.ActualRunning
				}
				if err := db.CreateGameServer(context.Background(), &server); err != nil {
					t.Fatal(err)
				}
			}
			payload, _ := json.Marshal(prepareRuntimeImageRequest{ProviderKey: tc.providerKey})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/runtime/images", bytes.NewReader(payload)))
			if recorder.Code != tc.wantStatus {
				t.Fatalf("got %d: %s", recorder.Code, recorder.Body.String())
			}
			for _, path := range []string{world, config, mod, installed, backup} {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, files[path]) {
					t.Fatalf("server data changed: %s, %v", path, err)
				}
			}
			for _, path := range []string{archive, archive + ".tmp", marker} {
				_, err := os.Stat(path)
				if tc.wantStatus == http.StatusOK && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("installation file remains: %s", path)
				}
				if tc.wantStatus != http.StatusOK && err != nil {
					t.Fatalf("failed uninstall removed %s", path)
				}
			}
			if tc.running && !strings.Contains(recorder.Body.String(), "Existing world") {
				t.Fatal("missing referencing server name")
			}
			if (tc.running || tc.wantStatus == http.StatusBadRequest || tc.wantStatus == http.StatusNotFound) && len(adapter.removed) != 0 {
				t.Fatal("Docker removal attempted despite failed guard")
			}
			if tc.wantStatus == http.StatusOK {
				if tc.referenced {
					stored, err := db.GetGameServer(context.Background(), "saved")
					if err != nil || stored.Spec.Version != "1.4.5.6" || stored.Spec.Config["worldName"] != "Existing world" || len(stored.Spec.ModIDs) != 1 || stored.Spec.ModIDs[0] != "existing-mod" {
						t.Fatalf("instance configuration not retained: %+v, %v", stored, err)
					}
					if len(adapter.stopped) != 1 || adapter.stopped[0].ServerID != "saved" || adapter.stopped[0].DataDir != filepath.Dir(filepath.Dir(world)) {
						t.Fatalf("wrong stopped container cleanup target: %+v", adapter.stopped)
					}
				}
				if len(adapter.removed) != 1 || adapter.removed[0] != "smartcat99999/terraria-vanilla:1.4.5.6" {
					t.Fatal("wrong image removed")
				}
				var status domain.RuntimeImageStatus
				if json.Unmarshal(recorder.Body.Bytes(), &status) != nil || status.Status != runtime.ImageStatusMissing {
					t.Fatal("expected missing runtime status")
				}
			}
		})
	}
}

func TestUninstallAndRestoreUsesLatestRuntimeWithoutResettingInstance(t *testing.T) {
	adapter := &uninstallTestAdapter{availableMockAdapter: availableMockAdapter{runtime.NewMockAdapter()}}
	router, db, cfg := newTestRouterWithAdapterAndInstallMarkers(t, adapter, true)
	dataDir := filepath.Join(cfg.DataDir, "instances", "kept")
	files := map[string]string{"Worlds/Existing.wld": "day 49", "Mods/kept.tmod": "existing mod", "serverconfig.txt": "existing configuration"}
	for name, value := range files {
		path := filepath.Join(dataDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	server := domain.GameServer{ID: "kept", Name: "Existing", ProviderKey: domain.ProviderTerrariaTModLoader,
		Spec:   domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped, Version: "v2026.02.3.1", Runtime: domain.ServerRuntimeSpec{DataDir: dataDir}, ModIDs: []string{"kept"}, Config: map[string]any{"serverName": "Existing", "worldName": "Existing", "worldSize": "medium", "difficulty": "classic", "maxPlayers": float64(8), "port": float64(7777), "language": "en-US", "autoCreateWorld": true}},
		Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped, ActualState: domain.ActualStopped},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	removed := request(http.MethodDelete, "/api/runtime/images", `{"providerKey":"terraria-tmodloader"}`)
	if removed.Code != http.StatusOK {
		t.Fatalf("uninstall: %d %s", removed.Code, removed.Body.String())
	}
	for _, kind := range []string{"runtime-installs", "runtime-images"} {
		if _, err := os.Stat(filepath.Join(cfg.DataDir, kind, "terraria-tmodloader")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("historical cache remains: %s", kind)
		}
	}
	startBeforeInstall := request(http.MethodPost, "/api/servers/kept/start", "")
	if startBeforeInstall.Code != http.StatusConflict {
		t.Fatalf("must require latest runtime before restore: %d %s", startBeforeInstall.Code, startBeforeInstall.Body.String())
	}
	prepared := request(http.MethodPost, "/api/runtime/images/prepare", `{"providerKey":"terraria-tmodloader"}`)
	if prepared.Code != http.StatusAccepted {
		t.Fatalf("prepare latest: %d %s", prepared.Code, prepared.Body.String())
	}
	marker := filepath.Join(cfg.DataDir, "runtime-installs", "terraria-tmodloader", "v2026.04.3.0.json")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec := request(http.MethodGet, "/api/games", "")
		var games []domain.GameCatalogEntry
		if err := json.Unmarshal(rec.Body.Bytes(), &games); err != nil {
			t.Fatal(err)
		}
		ready := false
		for _, game := range games {
			for _, p := range game.Providers {
				if p.Key == domain.ProviderTerrariaTModLoader {
					ready = p.RuntimeImage.Status == runtime.ImageStatusReady
				}
			}
		}
		if ready {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("latest runtime was not installed: %v", err)
	}
	started := request(http.MethodPost, "/api/servers/kept/start", "")
	if started.Code != http.StatusAccepted {
		t.Fatalf("restore: %d %s", started.Code, started.Body.String())
	}
	var restored domain.GameServer
	if err := json.Unmarshal(started.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ID != server.ID || restored.Spec.Version != "v2026.04.3.0" || restored.Spec.Runtime.DataDir != dataDir || restored.Spec.Config["worldName"] != "Existing" || len(restored.Spec.ModIDs) != 1 {
		t.Fatalf("wrong restored instance: %+v", restored)
	}
	for name, value := range files {
		data, err := os.ReadFile(filepath.Join(dataDir, name))
		if err != nil || string(data) != value {
			t.Fatalf("persistent data changed: %s, %v", name, err)
		}
	}
	old := request(http.MethodPost, "/api/runtime/images/prepare", `{"providerKey":"terraria-tmodloader","version":"v2026.02.3.1"}`)
	if old.Code != http.StatusBadRequest {
		t.Fatalf("must not reinstall old runtime: %d %s", old.Code, old.Body.String())
	}
}

func TestUninstallRuntimeImageRefusesSharedImageAcrossVersions(t *testing.T) {
	adapter := &uninstallTestAdapter{availableMockAdapter: availableMockAdapter{runtime.NewMockAdapter()}}
	router, db, _ := newTestRouterWithAdapterAndInstallMarkers(t, adapter, false)
	server := domain.GameServer{
		ID: "minecraft-saved", Name: "Minecraft stopped", ProviderKey: domain.ProviderMinecraft,
		Spec: domain.ServerSpec{Version: "1.21.4", DesiredState: domain.DesiredRunning},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/runtime/images", strings.NewReader(`{"providerKey":"minecraft","version":"1.21"}`)))
	if recorder.Code != http.StatusConflict || len(adapter.removed) != 0 {
		t.Fatalf("shared image must be protected: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestServerStoppedForImageRemovalRequiresConfirmedStop(t *testing.T) {
	for _, tc := range []struct {
		name    string
		desired domain.ServerDesiredState
		phase   domain.ServerPhase
		actual  domain.ServerActualState
		allowed bool
	}{
		{"stopped", domain.DesiredStopped, domain.PhaseStopped, domain.ActualStopped, true},
		{"container already absent", domain.DesiredStopped, domain.PhaseStopped, domain.ActualMissing, true},
		{"start queued", domain.DesiredRunning, domain.PhasePending, domain.ActualStopped, false},
		{"stopping", domain.DesiredStopped, domain.PhaseReconciling, domain.ActualRunning, false},
		{"unknown", domain.DesiredStopped, domain.PhaseStopped, domain.ActualUnknown, false},
		{"failed", domain.DesiredStopped, domain.PhaseFailed, domain.ActualStopped, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := domain.GameServer{Spec: domain.ServerSpec{DesiredState: tc.desired}, Status: domain.ServerRuntimeStatus{Phase: tc.phase, ActualState: tc.actual}}
			if serverStoppedForImageRemoval(server) != tc.allowed {
				t.Fatal("incorrect uninstall state guard")
			}
		})
	}
}
