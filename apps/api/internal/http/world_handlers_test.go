package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	worldsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/world"
)

func TestWorldImportListDownloadDuplicateAndDeleteEndpoints(t *testing.T) {
	router, _, cfg := newTestRouter(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "uploaded.wld")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("world-data")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/import", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(upload, request)
	if upload.Code != stdhttp.StatusCreated {
		t.Fatalf("expected world import 201, got %d: %s", upload.Code, upload.Body.String())
	}
	var imported domain.World
	if err := json.Unmarshal(upload.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	if imported.InstanceID != "unassigned" || imported.FileName != "uploaded.wld" {
		t.Fatalf("expected unassigned uploaded world, got %+v", imported)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "worlds", "unassigned", "uploaded.wld")); err != nil {
		t.Fatal(err)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected world list 200, got %d: %s", list.Code, list.Body.String())
	}
	var worlds []domain.World
	if err := json.Unmarshal(list.Body.Bytes(), &worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 || worlds[0].ID != imported.ID {
		t.Fatalf("expected listed uploaded world, got %+v", worlds)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds/"+imported.ID+"/download", nil))
	if download.Code != stdhttp.StatusOK {
		t.Fatalf("expected world download 200, got %d: %s", download.Code, download.Body.String())
	}
	if download.Body.String() != "world-data" {
		t.Fatalf("expected downloaded world content, got %q", download.Body.String())
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/"+imported.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected world delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "worlds", "unassigned", "uploaded.wld")); !os.IsNotExist(err) {
		t.Fatalf("expected imported world file deleted, stat err=%v", err)
	}
}

func TestWorldImportIsIdempotentForSameInstanceFile(t *testing.T) {
	router, db, _ := newTestRouter(t)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/worlds/import", "file", "uploaded.wld", []byte("world-v1")))
	if first.Code != stdhttp.StatusCreated {
		t.Fatalf("expected first world import 201, got %d: %s", first.Code, first.Body.String())
	}
	var firstWorld domain.World
	if err := json.Unmarshal(first.Body.Bytes(), &firstWorld); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/worlds/import", "file", "uploaded.wld", []byte("world-v2")))
	if second.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated world import 200, got %d: %s", second.Code, second.Body.String())
	}
	var secondWorld domain.World
	if err := json.Unmarshal(second.Body.Bytes(), &secondWorld); err != nil {
		t.Fatal(err)
	}
	if secondWorld.ID != firstWorld.ID {
		t.Fatalf("expected repeated world import to update existing world, got first=%+v second=%+v", firstWorld, secondWorld)
	}
	if secondWorld.SizeBytes != int64(len("world-v2")) {
		t.Fatalf("expected updated world size, got %+v", secondWorld)
	}
	worlds, err := db.ListWorlds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 {
		t.Fatalf("expected one world record after repeated import, got %+v", worlds)
	}
}

func TestDeleteWorldKeepsRecordWhenFileRemovalFails(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	worldDir := filepath.Join(cfg.DataDir, "worlds", "unassigned", "blocked.wld")
	if err := os.MkdirAll(filepath.Join(worldDir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	item := domain.World{
		ID:         "blocked-world",
		InstanceID: "unassigned",
		Name:       "Blocked",
		FileName:   "blocked.wld",
		SizeBytes:  1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &item); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/blocked-world", nil))
	if remove.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("expected world delete to fail when file removal fails, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetWorld(context.Background(), "blocked-world"); err != nil {
		t.Fatalf("expected world record to remain after failed file removal, got %v", err)
	}
}

func TestWorldImportRejectsUnknownInstance(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("instanceId", "missing-server"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "uploaded.wld")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("world-data")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/import", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(upload, request)
	if upload.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected unknown instance import 404, got %d: %s", upload.Code, upload.Body.String())
	}
	worlds, err := db.ListWorlds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 0 {
		t.Fatalf("expected no world records for unknown instance, got %+v", worlds)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "worlds", "missing-server", "uploaded.wld")); !os.IsNotExist(err) {
		t.Fatalf("expected no file for unknown instance import, stat err=%v", err)
	}
}

func TestWorldListPrunesMissingFilesAndDownloadReturnsJSONError(t *testing.T) {
	router, db, _ := newTestRouter(t)
	world := domain.World{
		ID:         "missing-world",
		InstanceID: "unassigned",
		Name:       "Missing",
		FileName:   "missing.wld",
		SizeBytes:  5,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds/missing-world/download", nil))
	if download.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected missing world download 404, got %d: %s", download.Code, download.Body.String())
	}
	if !strings.Contains(download.Body.String(), "world file not found on disk") {
		t.Fatalf("expected JSON missing file error, got %q", download.Body.String())
	}
	if _, err := db.GetWorld(context.Background(), world.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing world record deleted after download miss, got err=%v", err)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected world list 200, got %d: %s", list.Code, list.Body.String())
	}
	var worlds []domain.World
	if err := json.Unmarshal(list.Body.Bytes(), &worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 0 {
		t.Fatalf("expected missing world record to be pruned, got %+v", worlds)
	}
}

func TestResourceListsIncludeGameMetadata(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("pal-resource", cfg.DataDir)
	server.GameKey = domain.GamePalworld
	server.ProviderKey = domain.ProviderPalworld
	server.Port = palworld.DefaultInternalPort
	server.Config = terraria.Config{
		ServerName: "Palworld Server",
		WorldName:  "Palworld Save",
		MaxPlayers: 8,
		Port:       palworld.DefaultInternalPort,
		MOTD:       "admin-password",
	}
	server.ConfigPayload = map[string]any{
		"serverName":    server.Config.ServerName,
		"saveName":      server.Config.WorldName,
		"maxPlayers":    server.Config.MaxPlayers,
		"adminPassword": server.Config.MOTD,
	}
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte("palworld config"), 0o600); err != nil {
		t.Fatal(err)
	}
	createTestServer(t, db, server)
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(server.ID, "pal-save.wld", strings.NewReader("world-data")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:          "pal-world",
		InstanceID:  server.ID,
		ProviderKey: server.ProviderKey,
		Name:        "Pal Save",
		FileName:    "pal-save.wld",
		SizeBytes:   int64(len("world-data")),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	listWorlds := httptest.NewRecorder()
	router.ServeHTTP(listWorlds, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds", nil))
	if listWorlds.Code != stdhttp.StatusOK {
		t.Fatalf("expected world list 200, got %d: %s", listWorlds.Code, listWorlds.Body.String())
	}
	var worlds []domain.World
	if err := json.Unmarshal(listWorlds.Body.Bytes(), &worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 || worlds[0].GameKey != domain.GamePalworld || worlds[0].ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld metadata on world list, got %+v", worlds)
	}

	createBackup := httptest.NewRecorder()
	router.ServeHTTP(createBackup, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/backups", nil))
	if createBackup.Code != stdhttp.StatusCreated {
		t.Fatalf("expected backup create 201, got %d: %s", createBackup.Code, createBackup.Body.String())
	}
	var createdBackup domain.Backup
	if err := json.Unmarshal(createBackup.Body.Bytes(), &createdBackup); err != nil {
		t.Fatal(err)
	}
	if createdBackup.GameKey != domain.GamePalworld || createdBackup.ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld metadata on backup create, got %+v", createdBackup)
	}

	listBackups := httptest.NewRecorder()
	router.ServeHTTP(listBackups, httptest.NewRequest(stdhttp.MethodGet, "/api/backups", nil))
	if listBackups.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup list 200, got %d: %s", listBackups.Code, listBackups.Body.String())
	}
	var backups []domain.Backup
	if err := json.Unmarshal(listBackups.Body.Bytes(), &backups); err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].GameKey != domain.GamePalworld || backups[0].ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld metadata on backup list, got %+v", backups)
	}
}

func TestAssignWorldUpdatesServerConfigAndClearsContainer(t *testing.T) {
	t.Skip("obsolete: world assignment should update desired spec and let controller reconcile")
	router, db, cfg := newTestRouter(t)
	server := testServer("world-target", cfg.DataDir)
	server.ContainerID = "old-container"
	expectedWorldName := server.Config.WorldName
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	createTestServer(t, db, server)
	oldWorld := domain.World{
		ID:               "old-world",
		InstanceID:       server.ID,
		ActiveInstanceID: server.ID,
		Name:             server.WorldName,
		FileName:         server.WorldName + ".wld",
		SizeBytes:        5,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &oldWorld); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(server.ID, "new-home.wld", bytes.NewBufferString("world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:         "assign-world",
		InstanceID: server.ID,
		Name:       "new-home",
		FileName:   "new-home.wld",
		SizeBytes:  5,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	assign := httptest.NewRecorder()
	router.ServeHTTP(assign, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/assign-world/assign", bytes.NewBufferString(`{"instanceId":"world-target"}`)))
	if assign.Code != stdhttp.StatusOK {
		t.Fatalf("expected assign world 200, got %d: %s", assign.Code, assign.Body.String())
	}
	updated, err := loadTestServer(db, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.WorldName != expectedWorldName || updated.Config.WorldName != expectedWorldName || updated.ContainerID != "" {
		t.Fatalf("expected server world/config name to stay unchanged and cleared container, got %+v", updated)
	}
	if updated.SourceWorldID != world.ID || updated.SourceWorldName != world.Name {
		t.Fatalf("expected server source world to be recorded, got %+v", updated)
	}
	configBytes, err := os.ReadFile(filepath.Join(server.DataDir, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expectedConfigLine := "world=/home/container/Worlds/" + expectedWorldName + ".wld"
	if !bytes.Contains(configBytes, []byte(expectedConfigLine)) {
		t.Fatalf("expected serverconfig to point at assigned world, got %q", string(configBytes))
	}
	runtimeWorld, err := os.ReadFile(filepath.Join(server.DataDir, "Worlds", expectedWorldName+".wld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeWorld) != "world" {
		t.Fatalf("expected assigned world copied into runtime data dir, got %q", string(runtimeWorld))
	}
	previous, err := db.GetWorld(context.Background(), oldWorld.ID)
	if err != nil {
		t.Fatal(err)
	}
	if previous.ActiveInstanceID != "" {
		t.Fatalf("expected previous active world to be cleared, got %+v", previous)
	}
	assigned, err := db.GetWorld(context.Background(), world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.ActiveInstanceID != server.ID {
		t.Fatalf("expected assigned world to be active, got %+v", assigned)
	}
}

func TestAssignWorldAllowsReusableSnapshotForSameProvider(t *testing.T) {
	t.Skip("obsolete: world assignment should update desired spec and let controller reconcile")
	router, db, cfg := newTestRouter(t)
	source := testServer("snapshot-source", cfg.DataDir)
	target := testServer("snapshot-target", cfg.DataDir)
	expectedWorldName := target.Config.WorldName
	createTestServer(t, db, source)
	createTestServer(t, db, target)
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(source.ID, "shared.wld", bytes.NewBufferString("shared-world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:          "shared-world",
		InstanceID:  source.ID,
		ProviderKey: source.ProviderKey,
		Name:        "shared",
		FileName:    "shared.wld",
		SizeBytes:   12,
		Source:      "server_snapshot",
		Config:      testConfigPayload(source.Config),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	assign := httptest.NewRecorder()
	router.ServeHTTP(assign, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/shared-world/assign", bytes.NewBufferString(`{"instanceId":"snapshot-target"}`)))
	if assign.Code != stdhttp.StatusOK {
		t.Fatalf("expected same-provider snapshot assign 200, got %d: %s", assign.Code, assign.Body.String())
	}
	updated, err := loadTestServer(db, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.WorldName != expectedWorldName || updated.Config.WorldName != expectedWorldName {
		t.Fatalf("expected target server world name to stay unchanged, got %+v", updated)
	}
	if updated.SourceWorldID != world.ID || updated.SourceWorldName != world.Name {
		t.Fatalf("expected target server source world to be recorded, got %+v", updated)
	}
	got, err := os.ReadFile(filepath.Join(target.DataDir, "worlds", expectedWorldName+".wld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "shared-world" {
		t.Fatalf("expected snapshot materialized into target runtime, got %q", string(got))
	}
}

func TestAssignWorldRejectsSnapshotForDifferentProvider(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("snapshot-source", cfg.DataDir)
	target := testServer("snapshot-tmod", cfg.DataDir)
	target.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, source)
	createTestServer(t, db, target)
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(source.ID, "vanilla.wld", bytes.NewBufferString("vanilla-world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:          "vanilla-world",
		InstanceID:  source.ID,
		ProviderKey: source.ProviderKey,
		Name:        "vanilla",
		FileName:    "vanilla.wld",
		SizeBytes:   13,
		Source:      "server_snapshot",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	assign := httptest.NewRecorder()
	router.ServeHTTP(assign, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/vanilla-world/assign", bytes.NewBufferString(`{"instanceId":"snapshot-tmod"}`)))
	if assign.Code != stdhttp.StatusConflict {
		t.Fatalf("expected different-provider snapshot assign 409, got %d: %s", assign.Code, assign.Body.String())
	}
}

func TestDeleteActiveWorldRequiresUnassigningFirst(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("active-world-server", cfg.DataDir)
	server.WorldName = "active"
	server.Config.WorldName = "active"
	createTestServer(t, db, server)
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(server.ID, "active.wld", bytes.NewBufferString("world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:               "active-world",
		InstanceID:       server.ID,
		ActiveInstanceID: server.ID,
		Name:             "active",
		FileName:         "active.wld",
		SizeBytes:        5,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/active-world", nil))
	if remove.Code != stdhttp.StatusConflict {
		t.Fatalf("expected active world delete conflict, got %d: %s", remove.Code, remove.Body.String())
	}
	stored, err := db.GetWorld(context.Background(), world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != world.ID {
		t.Fatalf("expected active world to remain stored, got %+v", stored)
	}
}

func TestDeleteWorldRejectsTemplateUsedByServer(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("template-source", cfg.DataDir)
	target := testServer("template-target", cfg.DataDir)
	target.SourceWorldID = "template-world"
	target.SourceWorldName = "Template World"
	createTestServer(t, db, source)
	createTestServer(t, db, target)
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(source.ID, "template.wld", bytes.NewBufferString("template-world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:          "template-world",
		InstanceID:  source.ID,
		ProviderKey: source.ProviderKey,
		Name:        "Template World",
		FileName:    "template.wld",
		SizeBytes:   14,
		Source:      "server_snapshot",
		Config:      testConfigPayload(source.Config),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/template-world", nil))
	if remove.Code != stdhttp.StatusConflict {
		t.Fatalf("expected template delete 409 while in use, got %d: %s", remove.Code, remove.Body.String())
	}
}

func TestCreateWorldSnapshotFromServerRuntimeWorld(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("snapshot-source", cfg.DataDir)
	server.Name = "Snapshot Source"
	server.WorldName = server.Config.WorldName
	if err := os.MkdirAll(filepath.Join(server.DataDir, "Worlds"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "Worlds", server.Config.WorldName+".wld"), []byte("world-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	createTestServer(t, db, server)

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/snapshot-source/world-snapshots", bytes.NewBufferString(`{"name":"Reusable Snapshot"}`)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected world snapshot 201, got %d: %s", create.Code, create.Body.String())
	}
	var world domain.World
	if err := json.Unmarshal(create.Body.Bytes(), &world); err != nil {
		t.Fatal(err)
	}
	if world.Name != "Reusable Snapshot" || world.Source != "server_snapshot" || world.ActiveInstanceID != "" || world.ProviderKey != server.ProviderKey {
		t.Fatalf("expected server snapshot world record, got %+v", world)
	}
	if stringFromTestPayload(world.Config, "worldName", "") != server.Config.WorldName || intFromTestPayload(world.Config, "maxPlayers", 0) != server.Config.MaxPlayers {
		t.Fatalf("expected config snapshot, got %+v", world.Config)
	}
	path, err := worldsvc.NewService(cfg.DataDir).Path(server.ID, world.FileName)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world-data" {
		t.Fatalf("expected copied runtime world data, got %q", string(got))
	}
}

func TestCreateWorldSnapshotFindsVanillaRootWorldFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("snapshot-root-world", cfg.DataDir)
	server.WorldName = server.Config.WorldName
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, server.Config.WorldName+".wld"), []byte("root-world-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	createTestServer(t, db, server)

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/snapshot-root-world/world-snapshots", nil))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected world snapshot 201, got %d: %s", create.Code, create.Body.String())
	}
	var world domain.World
	if err := json.Unmarshal(create.Body.Bytes(), &world); err != nil {
		t.Fatal(err)
	}
	path, err := worldsvc.NewService(cfg.DataDir).Path(server.ID, world.FileName)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "root-world-data" {
		t.Fatalf("expected copied root world data, got %q", string(got))
	}
}
