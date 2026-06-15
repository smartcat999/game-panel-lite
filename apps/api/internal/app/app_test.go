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
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestInvalidDockerHostKeepsAPIAvailableButStartFails(t *testing.T) {
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
	if create.Code != http.StatusCreated {
		t.Fatalf("expected create server to keep working without Docker, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServerInstance
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	start := httptest.NewRecorder()
	api.Routes().ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != http.StatusAccepted {
		t.Fatalf("expected async start to be accepted without blocking on Docker runtime, got %d: %s", start.Code, start.Body.String())
	}
	failed := waitForAPIServerStatus(t, api, server.ID, domain.StatusErrored)
	if failed.ContainerID != "" {
		t.Fatalf("expected failed start to keep an empty container id, got %+v", failed)
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
	server := domain.GameServerInstance{
		ID:          "existing",
		Name:        "Existing",
		GameKey:     "terraria",
		ProviderKey: domain.ProviderTerrariaVanilla,
		Status:      domain.StatusRunning,
		ContainerID: "real-container",
		WorldName:   "ExistingWorld",
		Port:        7777,
		MaxPlayers:  8,
		DataDir:     filepath.Join(cfg.DataDir, "instances", "existing"),
		Config:      terraria.Presets[0].Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateServer(t.Context(), &server); err != nil {
		t.Fatal(err)
	}
	api, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	stop := httptest.NewRecorder()
	api.Routes().ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/servers/existing/stop", nil))
	if stop.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected stop to fail without Docker runtime, got %d: %s", stop.Code, stop.Body.String())
	}
	if !strings.Contains(stop.Body.String(), "Docker runtime unavailable") {
		t.Fatalf("expected Docker runtime unavailable message, got %q", stop.Body.String())
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
	server := domain.GameServerInstance{
		ID:          "existing-delete",
		Name:        "Existing Delete",
		GameKey:     "terraria",
		ProviderKey: domain.ProviderTerrariaVanilla,
		Status:      domain.StatusRunning,
		ContainerID: "real-container",
		WorldName:   "ExistingWorld",
		Port:        7777,
		MaxPlayers:  8,
		DataDir:     filepath.Join(cfg.DataDir, "instances", "existing-delete"),
		Config:      terraria.Presets[0].Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateServer(t.Context(), &server); err != nil {
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
	server = waitForAPIServerStatus(t, api, "existing-delete", domain.StatusErrored)
	if server.ContainerID != "real-container" {
		t.Fatalf("expected failed delete to preserve existing container id, got %+v", server)
	}
	if _, err := db.GetServer(t.Context(), "existing-delete"); err != nil {
		t.Fatalf("expected server record to remain after failed delete, got %v", err)
	}
}

func waitForAPIServerStatus(t *testing.T, api *App, id string, status domain.ServerStatus) domain.GameServerInstance {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		recorder := httptest.NewRecorder()
		api.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/servers/"+id, nil))
		if recorder.Code == http.StatusOK {
			var server domain.GameServerInstance
			if err := json.Unmarshal(recorder.Body.Bytes(), &server); err != nil {
				t.Fatal(err)
			}
			if server.Status == status {
				return server
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	recorder := httptest.NewRecorder()
	api.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/servers/"+id, nil))
	t.Fatalf("expected server %s to reach %s, got %d: %s", id, status, recorder.Code, recorder.Body.String())
	return domain.GameServerInstance{}
}
