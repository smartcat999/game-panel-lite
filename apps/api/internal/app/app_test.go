package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestInvalidDockerHostKeepsAPIAvailableButCreateFails(t *testing.T) {
	root := t.TempDir()
	api, err := New(config.Config{
		Host:       "127.0.0.1",
		Port:       "4000",
		DataDir:    filepath.Join(root, "data"),
		DBPath:     filepath.Join(root, "gamepanel.db"),
		DockerHost: "bad://daemon",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	createPayload := `{
		"name":"Runtime unavailable",
		"providerKey":"terraria-vanilla",
		"config":{
			"serverName":"Runtime unavailable",
			"worldName":"RuntimeWorld",
			"worldSize":"medium",
			"worldEvil":"random",
			"difficulty":"classic",
			"maxPlayers":8,
			"port":7777,
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	create := httptest.NewRecorder()
	api.Routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBufferString(createPayload)))
	if create.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected create server to fail fast without Docker, got %d: %s", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "Docker runtime unavailable") {
		t.Fatalf("expected runtime availability error, got %s", create.Body.String())
	}
}

func TestInvalidDockerHostDoesNotMockStopExistingContainer(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		Host:       "127.0.0.1",
		Port:       "4000",
		DataDir:    filepath.Join(root, "data"),
		DBPath:     filepath.Join(root, "gamepanel.db"),
		DockerHost: "bad://daemon",
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	server := testRunningGameServer("existing", cfg.DataDir)
	if err := db.CreateGameServer(t.Context(), &server); err != nil {
		t.Fatal(err)
	}
	api, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	stop := httptest.NewRecorder()
	api.Routes().ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/servers/existing/stop", nil))
	if stop.Code != http.StatusAccepted {
		t.Fatalf("expected stop command to be accepted without blocking on Docker runtime, got %d: %s", stop.Code, stop.Body.String())
	}
	server = waitForAPIServerPhase(t, api, "existing", domain.PhaseFailed)
	if server.Status.RuntimeID != "real-container" {
		t.Fatalf("expected failed stop to preserve existing container id, got %+v", server)
	}
}

func TestInvalidDockerHostDoesNotDeleteExistingContainerRecord(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		Host:       "127.0.0.1",
		Port:       "4000",
		DataDir:    filepath.Join(root, "data"),
		DBPath:     filepath.Join(root, "gamepanel.db"),
		DockerHost: "bad://daemon",
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	server := testRunningGameServer("existing-delete", cfg.DataDir)
	server.Name = "Existing Delete"
	if err := db.CreateGameServer(t.Context(), &server); err != nil {
		t.Fatal(err)
	}
	api, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	remove := httptest.NewRecorder()
	api.Routes().ServeHTTP(remove, httptest.NewRequest(http.MethodDelete, "/api/servers/existing-delete", nil))
	if remove.Code != http.StatusAccepted {
		t.Fatalf("expected async delete to be accepted without blocking on Docker runtime, got %d: %s", remove.Code, remove.Body.String())
	}
	server = waitForAPIServerPhase(t, api, "existing-delete", domain.PhaseFailed)
	if server.Status.RuntimeID != "real-container" {
		t.Fatalf("expected failed delete to preserve existing container id, got %+v", server)
	}
	if _, err := db.GetGameServer(t.Context(), "existing-delete"); err != nil {
		t.Fatalf("expected server record to remain after failed delete, got %v", err)
	}
}

func testRunningGameServer(id string, dataDir string) domain.GameServer {
	now := time.Now()
	return domain.GameServer{
		ID:          id,
		Name:        "Existing",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredRunning,
			Version:      "1.4.5.6",
			Config: map[string]any{
				"serverName":      "Existing",
				"worldName":       "ExistingWorld",
				"worldSize":       "medium",
				"worldEvil":       "random",
				"difficulty":      "classic",
				"maxPlayers":      8,
				"port":            7777,
				"secure":          true,
				"language":        "en-US",
				"autoCreateWorld": true,
			},
			Network: domain.ServerNetworkSpec{Port: 7777, HostPort: 7777},
			Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(dataDir, "instances", id)},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhaseRunning,
			ActualState:        domain.ActualRunning,
			RuntimeID:          "real-container",
			ObservedGeneration: 1,
			AppliedGeneration:  1,
			LastTransitionAt:   now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func waitForAPIServerPhase(t *testing.T, api *App, id string, phase domain.ServerPhase) domain.GameServer {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		recorder := httptest.NewRecorder()
		api.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/servers/"+id, nil))
		if recorder.Code == http.StatusOK {
			var server domain.GameServer
			if err := json.Unmarshal(recorder.Body.Bytes(), &server); err != nil {
				t.Fatal(err)
			}
			if server.Status.Phase == phase {
				return server
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	recorder := httptest.NewRecorder()
	api.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/servers/"+id, nil))
	t.Fatalf("expected server %s to reach phase %s, got %d: %s", id, phase, recorder.Code, recorder.Body.String())
	return domain.GameServer{}
}
