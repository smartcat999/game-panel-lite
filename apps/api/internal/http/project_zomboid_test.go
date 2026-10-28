package http

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/projectzomboid"
)

func TestPZCreateReservesBothUDPPorts(t *testing.T) {
	router, _, _ := newTestRouter(t)
	create := func(port int) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"providerKey": "project-zomboid", "hostPort": port, "config": map[string]any{"server": map[string]any{"PublicName": "PZ test", "MaxPlayers": 2}}})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewReader(body)))
		return response
	}
	response := create(31000)
	if response.Code != stdhttp.StatusCreated {
		t.Fatalf("create %d: %s", response.Code, response.Body.String())
	}
	var server domain.GameServer
	_ = json.Unmarshal(response.Body.Bytes(), &server)
	if server.GameKey != domain.GameProjectZomboid || server.Spec.Network.Port != 16261 {
		t.Fatal("invalid game routing")
	}
	for _, port := range []int{30999, 31000, 31001, 65535} {
		if r := create(port); r.Code != stdhttp.StatusBadRequest {
			t.Fatalf("accepted overlapping or invalid port %d: %d", port, r.Code)
		}
	}
	if r := create(31002); r.Code != stdhttp.StatusCreated {
		t.Fatalf("adjacent free range rejected: %d %s", r.Code, r.Body.String())
	}
}

func TestPZBackupRestoresWholeZomboidSubtreeOnly(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	p := projectzomboid.NewProvider()
	server := domain.GameServer{ID: "pz-backup", Name: "PZ", GameKey: p.GameKey(), ProviderKey: p.Key(), Spec: domain.ServerSpec{Config: p.DefaultConfigPayload(), DesiredState: domain.DesiredStopped, Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(cfg.DataDir, "instances", "pz-backup")}}, Status: domain.ServerRuntimeStatus{ActualState: domain.ActualStopped, Phase: domain.PhaseStopped}}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	dir := server.Spec.Runtime.DataDir
	files := map[string]string{"Zomboid/Saves/Multiplayer/gamepanel/map_t.bin": "saved world", "Zomboid/db/gamepanel.db": "player accounts", "Zomboid/Server/gamepanel.ini": "identity config", "game.bin": "shared program marker"}
	for name, value := range files {
		path := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/backups", nil))
	if r.Code != stdhttp.StatusCreated {
		t.Fatalf("backup: %d %s", r.Code, r.Body.String())
	}
	var backup domain.Backup
	_ = json.Unmarshal(r.Body.Bytes(), &backup)
	archive, err := zip.OpenReader(filepath.Join(cfg.DataDir, "backups", server.ID, backup.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 3 {
		t.Fatal("backup missed save/db/config or included game binaries")
	}
	archive.Close()
	world := filepath.Join(dir, "Zomboid/Saves/Multiplayer/gamepanel/map_t.bin")
	_ = os.WriteFile(world, []byte("later world"), 0600)
	extra := filepath.Join(dir, "Zomboid/Saves/Multiplayer/gamepanel/after-backup.bin")
	_ = os.WriteFile(extra, []byte("later region"), 0600)
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/"+backup.ID+"/restore", nil))
	if r.Code != stdhttp.StatusOK {
		t.Fatalf("restore: %d %s", r.Code, r.Body.String())
	}
	for name, want := range files {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != want {
			t.Fatalf("did not restore/preserve %s", name)
		}
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Fatal("restore retained a later world region")
	}
}

func TestPZPartialConfigRetainsServerIdentity(t *testing.T) {
	p := projectzomboid.NewProvider()
	current := p.DefaultConfigPayload()
	next, _, err := decodeProviderConfigPayload(p, json.RawMessage(`{"server":{"PublicName":"Renamed"}}`), current)
	if err != nil {
		t.Fatal(err)
	}
	oldServer := current["server"].(map[string]any)
	newServer := next["server"].(map[string]any)
	for _, key := range []string{"ResetID", "ServerPlayerID"} {
		if oldServer[key] != newServer[key] {
			t.Errorf("partial update replaced %s", key)
		}
	}
	if oldServer["PublicName"] == "Renamed" {
		t.Fatal("merge mutated stored config")
	}
	if err := p.ValidateConfigUpdate(current, next); err != nil {
		t.Fatal(err)
	}
}

func TestPZConfigPresetRemovesNestedCredentialsAndIdentity(t *testing.T) {
	p := projectzomboid.NewProvider()
	input := p.DefaultConfigPayload()
	server := input["server"].(map[string]any)
	server["Password"] = "private-join-password"
	server["DiscordToken"] = "private-discord-token"
	data, _ := json.Marshal(input)
	text, err := sanitizePresetConfigPayload(p, string(data))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	_ = json.Unmarshal([]byte(text), &result)
	clean := result["server"].(map[string]any)
	for _, name := range []string{"Password", "DiscordToken", "ResetID", "ServerPlayerID"} {
		if _, exists := clean[name]; exists {
			t.Errorf("preset retained %s", name)
		}
	}
	if _, exists := result["adminPassword"]; exists {
		t.Fatal("preset retained bootstrap password")
	}
	if server["Password"] != "private-join-password" {
		t.Fatal("sanitization mutated original config")
	}
}
