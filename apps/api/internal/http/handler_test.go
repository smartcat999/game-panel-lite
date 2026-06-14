package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	worldsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/world"
)

func newTestRouter(t *testing.T) (stdhttp.Handler, *store.Store, config.Config) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		Host:       "127.0.0.1",
		Port:       "4000",
		DataDir:    filepath.Join(root, "data"),
		DBPath:     filepath.Join(root, "gamepanel.db"),
		DockerHost: "unix:///initial.sock",
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	runtimeAdapter := runtime.NewSwitchableAdapter(runtime.NewMockAdapter())
	monitor := runtime.NewDockerMonitor(runtimeAdapter)
	monitor.Refresh(context.Background())
	handler := NewHandler(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		db,
		provider.NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider()),
		runtimeAdapter,
		monitor,
		func(string) (runtime.Adapter, error) { return runtime.NewMockAdapter(), nil },
	)
	router := chi.NewRouter()
	handler.Register(router)
	return router, db, cfg
}

func TestServerLifecycleAndLogEndpoints(t *testing.T) {
	router, _, _ := newTestRouter(t)
	createPayload := `{
		"name":"Vanilla Test",
		"providerKey":"terraria-vanilla",
		"config":{
			"serverName":"Vanilla Test",
			"worldName":"TestWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":8,
			"port":7777,
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(createPayload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServerInstance
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.ID == "" || server.ContainerID != "" || server.ProviderKey != domain.ProviderTerrariaVanilla {
		t.Fatalf("expected created vanilla server record without fixed container, got %+v", server)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusOK {
		t.Fatalf("expected start 200, got %d: %s", start.Code, start.Body.String())
	}
	var started domain.GameServerInstance
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Status != domain.StatusRunning || started.ContainerID == "" {
		t.Fatalf("expected start to create runtime container and mark running, got %+v", started)
	}

	command := httptest.NewRecorder()
	router.ServeHTTP(command, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/command", bytes.NewBufferString(`{"command":"say hello"}`)))
	if command.Code != stdhttp.StatusOK {
		t.Fatalf("expected command 200, got %d: %s", command.Code, command.Body.String())
	}

	restart := httptest.NewRecorder()
	router.ServeHTTP(restart, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/restart", nil))
	if restart.Code != stdhttp.StatusOK {
		t.Fatalf("expected restart 200, got %d: %s", restart.Code, restart.Body.String())
	}
	var restarted domain.GameServerInstance
	if err := json.Unmarshal(restart.Body.Bytes(), &restarted); err != nil {
		t.Fatal(err)
	}
	if restarted.Status != domain.StatusRunning {
		t.Fatalf("expected restart running, got %+v", restarted)
	}

	stop := httptest.NewRecorder()
	router.ServeHTTP(stop, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/stop", nil))
	if stop.Code != stdhttp.StatusOK {
		t.Fatalf("expected stop 200, got %d: %s", stop.Code, stop.Body.String())
	}
	var stopped domain.GameServerInstance
	if err := json.Unmarshal(stop.Body.Bytes(), &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != domain.StatusStopped {
		t.Fatalf("expected stop status stopped, got %+v", stopped)
	}

	logs := httptest.NewRecorder()
	router.ServeHTTP(logs, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/logs", nil))
	if logs.Code != stdhttp.StatusOK {
		t.Fatalf("expected logs 200, got %d: %s", logs.Code, logs.Body.String())
	}
	if got := logs.Body.String(); !bytes.Contains([]byte(got), []byte("event: log")) || !bytes.Contains([]byte(got), []byte("Mock Terraria log stream")) {
		t.Fatalf("expected SSE log event, got %q", got)
	}

	activity := httptest.NewRecorder()
	router.ServeHTTP(activity, httptest.NewRequest(stdhttp.MethodGet, "/api/activity", nil))
	if activity.Code != stdhttp.StatusOK {
		t.Fatalf("expected activity 200, got %d: %s", activity.Code, activity.Body.String())
	}
	var events []domain.ActivityEvent
	if err := json.Unmarshal(activity.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected lifecycle actions to create activity events")
	}
	if events[0].Type == "" || events[0].Message == "" {
		t.Fatalf("expected populated activity event, got %+v", events[0])
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/"+server.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
}

func TestTModLoaderModUploadListAndDeleteEndpoints(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "example.tmod")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("mod")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/servers/tmod/mods/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(upload, request)
	if upload.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod upload 201, got %d: %s", upload.Code, upload.Body.String())
	}
	var mod domain.ModFile
	if err := json.Unmarshal(upload.Body.Bytes(), &mod); err != nil {
		t.Fatal(err)
	}
	if mod.FileName != "example.tmod" || !mod.Enabled {
		t.Fatalf("expected uploaded enabled mod, got %+v", mod)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod/mods", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod list 200, got %d: %s", list.Code, list.Body.String())
	}
	var mods []domain.ModFile
	if err := json.Unmarshal(list.Body.Bytes(), &mods); err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || mods[0].ID != mod.ID {
		t.Fatalf("expected listed uploaded mod, got %+v", mods)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/tmod/mods/"+mod.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "mods", "tmod", "example.tmod")); !os.IsNotExist(err) {
		t.Fatalf("expected mod file deleted, stat err=%v", err)
	}
}

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

	duplicate := httptest.NewRecorder()
	router.ServeHTTP(duplicate, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/"+imported.ID+"/duplicate", bytes.NewBufferString(`{"name":"Copy","fileName":"copy.wld"}`)))
	if duplicate.Code != stdhttp.StatusCreated {
		t.Fatalf("expected world duplicate 201, got %d: %s", duplicate.Code, duplicate.Body.String())
	}
	var copied domain.World
	if err := json.Unmarshal(duplicate.Body.Bytes(), &copied); err != nil {
		t.Fatal(err)
	}
	if copied.Name != "Copy" || copied.FileName != "copy.wld" {
		t.Fatalf("expected copied world metadata, got %+v", copied)
	}
	copiedBytes, err := os.ReadFile(filepath.Join(cfg.DataDir, "worlds", "unassigned", "copy.wld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copiedBytes) != "world-data" {
		t.Fatalf("expected copied world content, got %q", string(copiedBytes))
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/"+copied.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected world delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "worlds", "unassigned", "copy.wld")); !os.IsNotExist(err) {
		t.Fatalf("expected copied world file deleted, stat err=%v", err)
	}
}

func TestBackupCreateListDownloadRestoreAndDeleteEndpoints(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("backup-source", cfg.DataDir)
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(server.DataDir, "serverconfig.txt")
	if err := os.WriteFile(configPath, []byte("original-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/backup-source/backups", nil))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected backup create 201, got %d: %s", create.Code, create.Body.String())
	}
	var backup domain.Backup
	if err := json.Unmarshal(create.Body.Bytes(), &backup); err != nil {
		t.Fatal(err)
	}
	if backup.InstanceID != "backup-source" || backup.FileName == "" {
		t.Fatalf("expected created backup metadata, got %+v", backup)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/backups", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup list 200, got %d: %s", list.Code, list.Body.String())
	}
	var backups []domain.Backup
	if err := json.Unmarshal(list.Body.Bytes(), &backups); err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].ID != backup.ID {
		t.Fatalf("expected listed created backup, got %+v", backups)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/backups/"+backup.ID+"/download", nil))
	if download.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup download 200, got %d: %s", download.Code, download.Body.String())
	}
	if download.Body.Len() == 0 {
		t.Fatal("expected non-empty backup archive")
	}

	if err := os.WriteFile(configPath, []byte("mutated-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := httptest.NewRecorder()
	router.ServeHTTP(restore, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/"+backup.ID+"/restore", nil))
	if restore.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup restore 200, got %d: %s", restore.Code, restore.Body.String())
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "original-config" {
		t.Fatalf("expected restored config, got %q", string(restored))
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/backups/"+backup.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "backups", "backup-source", backup.FileName)); !os.IsNotExist(err) {
		t.Fatalf("expected backup archive deleted, stat err=%v", err)
	}
}

func TestSettingsEndpointsReadAndUpdateDockerHost(t *testing.T) {
	router, _, _ := newTestRouter(t)

	read := httptest.NewRecorder()
	router.ServeHTTP(read, httptest.NewRequest(stdhttp.MethodGet, "/api/settings", nil))
	if read.Code != stdhttp.StatusOK {
		t.Fatalf("expected settings read 200, got %d: %s", read.Code, read.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(read.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["dockerHost"] != "unix:///initial.sock" {
		t.Fatalf("expected initial docker host, got %q", got["dockerHost"])
	}

	body := bytes.NewBufferString(`{"dockerHost":"unix:///updated.sock"}`)
	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/settings", body))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected settings update 200, got %d: %s", update.Code, update.Body.String())
	}
	if err := json.Unmarshal(update.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["dockerHost"] != "unix:///updated.sock" {
		t.Fatalf("expected updated docker host, got %q", got["dockerHost"])
	}
}

func TestMigrateWorldEndpointCopiesToTargetServer(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("source", cfg.DataDir)
	target := testServer("target", cfg.DataDir)
	if err := db.CreateServer(context.Background(), &source); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import("source", "journey.wld", bytes.NewBufferString("world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{ID: "world-1", InstanceID: "source", Name: "Journey", FileName: "journey.wld", SizeBytes: 5, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/world-1/migrate", bytes.NewBufferString(`{"instanceId":"target"}`)))
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected migrate world 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var migrated domain.World
	if err := json.Unmarshal(recorder.Body.Bytes(), &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.InstanceID != "target" || migrated.ActiveInstanceID != "target" {
		t.Fatalf("expected migrated world target, got %+v", migrated)
	}
	got, err := os.ReadFile(filepath.Join(cfg.DataDir, "worlds", "target", "journey.wld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world" {
		t.Fatalf("expected migrated world content, got %q", string(got))
	}
}

func TestMigrateBackupEndpointCopiesToTargetServer(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("source", cfg.DataDir)
	target := testServer("target", cfg.DataDir)
	if err := os.MkdirAll(source.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source.DataDir, "serverconfig.txt"), []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &source); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	backupPath, size, err := backupsvc.NewService(cfg.DataDir).Create("source", source.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	backup := domain.Backup{ID: "backup-1", InstanceID: "source", FileName: filepath.Base(backupPath), WorldName: "Source World", SizeBytes: size, Type: "Manual", CreatedAt: time.Now()}
	if err := db.CreateBackup(context.Background(), &backup); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/backup-1/migrate", bytes.NewBufferString(`{"instanceId":"target"}`)))
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected migrate backup 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var migrated domain.Backup
	if err := json.Unmarshal(recorder.Body.Bytes(), &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.InstanceID != "target" || migrated.WorldName != target.WorldName {
		t.Fatalf("expected migrated backup target, got %+v", migrated)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "backups", "target", filepath.Base(backupPath))); err != nil {
		t.Fatal(err)
	}
}

func testServer(id string, dataDir string) domain.GameServerInstance {
	return domain.GameServerInstance{
		ID:          id,
		Name:        id + " server",
		GameKey:     "terraria",
		ProviderKey: domain.ProviderTerrariaVanilla,
		Status:      domain.StatusStopped,
		WorldName:   id + " world",
		Port:        7777,
		MaxPlayers:  8,
		DataDir:     filepath.Join(dataDir, "instances", id),
		Config:      terraria.Presets[0].Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}
