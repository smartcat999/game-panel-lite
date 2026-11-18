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
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/sevendaystodie"
)

func TestSevenDaysCreateReservesWholePortRange(t *testing.T) {
	router, db, _ := newTestRouter(t)
	create := func(port int) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"providerKey": "seven-days-to-die", "hostPort": port, "config": sevendaystodie.NewProvider().DefaultConfigPayload()})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewReader(body)))
		return response
	}
	first := create(30000)
	if first.Code != stdhttp.StatusCreated {
		t.Fatalf("create: %d %s", first.Code, first.Body.String())
	}
	var server domain.GameServer
	if err := json.Unmarshal(first.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.GameKey != domain.GameSevenDaysToDie || server.Spec.Network.Port != 26900 {
		t.Fatalf("wrong provider config: %+v", server)
	}
	for _, port := range []int{29999, 30000, 30001, 30002, 65534} {
		if response := create(port); response.Code != stdhttp.StatusBadRequest {
			t.Fatalf("overlapping/overflow range %d: %d %s", port, response.Code, response.Body.String())
		}
	}
	if response := create(30003); response.Code != stdhttp.StatusCreated {
		t.Fatalf("adjacent free range: %d %s", response.Code, response.Body.String())
	}
	if _, err := db.GetGameServer(context.Background(), server.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSevenDaysSaveOnlyBackupRestore(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	p := sevendaystodie.NewProvider()
	server := domain.GameServer{ID: "seven-days-backup", Name: "Friends", Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(cfg.DataDir, "instances", "seven-days-backup")}}}
	server.GameKey, server.ProviderKey, server.Spec.Config = p.GameKey(), p.Key(), p.DefaultConfigPayload()
	server.Spec.DesiredState, server.Status.ActualState, server.Status.Phase = domain.DesiredStopped, domain.ActualStopped, domain.PhaseStopped
	if err := os.MkdirAll(server.Spec.Runtime.DataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	dir := server.Spec.Runtime.DataDir
	if err := os.MkdirAll(filepath.Join(dir, "userdata", "Saves"), 0755); err != nil {
		t.Fatal(err)
	}
	savePath := filepath.Join(dir, "userdata", "Saves", "world.dat")
	if err := os.WriteFile(savePath, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.bin"), []byte("large game files"), 0600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/backups", nil))
	if response.Code != stdhttp.StatusCreated {
		t.Fatalf("backup: %d %s", response.Code, response.Body.String())
	}
	var item domain.Backup
	if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(filepath.Join(cfg.DataDir, "backups", server.ID, item.FileName))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 1 || archive.File[0].Name != "userdata/Saves/world.dat" {
		t.Fatalf("unexpected backup contents: %+v", archive.File)
	}
	if err := os.WriteFile(savePath, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	newRegion := filepath.Join(dir, "userdata", "Saves", "new-region.dat")
	if err := os.WriteFile(newRegion, []byte("created after backup"), 0600); err != nil {
		t.Fatal(err)
	}
	restore := httptest.NewRecorder()
	router.ServeHTTP(restore, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/"+item.ID+"/restore", nil))
	if restore.Code != stdhttp.StatusOK {
		t.Fatalf("restore: %d %s", restore.Code, restore.Body.String())
	}
	content, err := os.ReadFile(savePath)
	if err != nil || string(content) != "before" {
		t.Fatalf("save=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "game.bin")); err != nil {
		t.Fatalf("restore removed game files: %v", err)
	}
	if _, err := os.Stat(newRegion); !os.IsNotExist(err) {
		t.Fatal("restore retained world regions created after backup")
	}
}

func TestSevenDaysSteamUpdateRequest(t *testing.T) {
	h, _, dataDir := newGameUpdateUnitHandler(t, newGameUpdateHTTPAdapter())
	p := sevendaystodie.NewProvider()
	h.provider = provider.NewRegistry(p)
	server := domain.GameServer{ID: "seven-days-update", ProviderKey: p.Key(), Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: dataDir}}}
	request, err := h.gameUpdateRequest(server, "job-seven-days")
	if err != nil {
		t.Fatal(err)
	}
	if request.AppID != "294420" || request.InstallDir != "/data" || request.Executable != "7DaysToDieServer.x86_64" {
		t.Fatalf("wrong update request: %+v", request)
	}
	check, err := h.providerGameUpdateCheckRequest(p.Key(), "check-seven-days")
	if err != nil || check.AppID != "294420" || check.DataDir != "" {
		t.Fatalf("unsafe metadata check: %+v %v", check, err)
	}
}

func TestSevenDaysAdvancedConfigPersistsOnLegacyServer(t *testing.T) {
	router, db, _ := newTestRouter(t)
	server := domain.GameServer{
		ID: "seven-settings", Name: "Legacy save", GameKey: domain.GameSevenDaysToDie, ProviderKey: domain.ProviderSevenDaysToDie,
		Spec: domain.ServerSpec{DesiredState: domain.DesiredStopped, Generation: 1,
			Config:  map[string]any{"worldName": "原有存档", "worldGenSeed": "GamePanelLite", "worldGenSize": 4096},
			Network: domain.ServerNetworkSpec{Port: 26900, HostPort: 30000}},
		Status: domain.ServerRuntimeStatus{ActualState: domain.ActualStopped, Phase: domain.PhaseStopped},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	update := func(config map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]any{"config": config})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/"+server.ID+"/config", bytes.NewReader(body)))
		return response
	}
	response := update(map[string]any{"region": "Asia", "language": "Chinese", "serverMaxAllowedViewDistance": 8, "dynamicMeshEnabled": false, "landClaimOnlineDurabilityModifier": 0, "landClaimOfflineDurabilityModifier": 2})
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("update: %d %s", response.Code, response.Body.String())
	}
	saved, err := db.GetGameServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{"region": "Asia", "language": "Chinese", "serverMaxAllowedViewDistance": float64(8), "dynamicMeshEnabled": false, "landClaimOnlineDurabilityModifier": float64(0), "landClaimOfflineDurabilityModifier": float64(2), "worldName": "原有存档", "worldGenSeed": "GamePanelLite", "worldGenSize": float64(4096)} {
		if saved.Spec.Config[k] != want {
			t.Errorf("%s: got %v want %v", k, saved.Spec.Config[k], want)
		}
	}
	if _, ok := saved.Spec.Config["landClaimSize"]; ok {
		t.Fatal("partial update invented an unmanaged setting")
	}
	if response := update(map[string]any{"serverMaxAllowedViewDistance": 13}); response.Code != stdhttp.StatusBadRequest {
		t.Fatalf("invalid view distance: %d", response.Code)
	}
	after, err := db.GetGameServer(context.Background(), server.ID)
	if err != nil || after.Spec.Config["serverMaxAllowedViewDistance"] != float64(8) {
		t.Fatal("failed update changed persisted settings")
	}
}
