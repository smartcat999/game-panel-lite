package http

import (
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
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

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
	createTestServer(t, db, server)

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

func TestServerSavesEndpointsAreGameAware(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("minecraft-saves", cfg.DataDir)
	server.GameKey = domain.GameMinecraft
	server.ProviderKey = domain.ProviderMinecraft
	server.Port = 25565
	server.DataDir = filepath.Join(cfg.DataDir, "instances", "minecraft-saves")
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "level.dat"), []byte("world-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	createTestServer(t, db, server)

	snapshot := httptest.NewRecorder()
	router.ServeHTTP(snapshot, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/minecraft-saves/saves/snapshot", nil))
	if snapshot.Code != stdhttp.StatusCreated {
		t.Fatalf("expected save snapshot 201, got %d: %s", snapshot.Code, snapshot.Body.String())
	}
	var snapshotResp struct {
		SaveDisplayName string        `json:"saveDisplayName"`
		Save            domain.Backup `json:"save"`
	}
	if err := json.Unmarshal(snapshot.Body.Bytes(), &snapshotResp); err != nil {
		t.Fatal(err)
	}
	if snapshotResp.SaveDisplayName != "world" {
		t.Fatalf("expected Minecraft save display name 'world', got %q", snapshotResp.SaveDisplayName)
	}
	if snapshotResp.Save.InstanceID != "minecraft-saves" {
		t.Fatalf("expected snapshot tied to server, got %+v", snapshotResp.Save)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/minecraft-saves/saves", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected saves list 200, got %d: %s", list.Code, list.Body.String())
	}
	var listResp struct {
		SaveDisplayName string          `json:"saveDisplayName"`
		Saves           []domain.Backup `json:"saves"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if listResp.SaveDisplayName != "world" || len(listResp.Saves) != 1 {
		t.Fatalf("expected one save with game-aware name, got %+v", listResp)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/minecraft-saves/saves/"+snapshotResp.Save.ID+"/download", nil))
	if download.Code != stdhttp.StatusOK {
		t.Fatalf("expected save download 200, got %d: %s", download.Code, download.Body.String())
	}
	if download.Body.Len() == 0 {
		t.Fatal("expected non-empty save archive")
	}

	restore := httptest.NewRecorder()
	router.ServeHTTP(restore, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/minecraft-saves/saves/"+snapshotResp.Save.ID+"/restore", nil))
	if restore.Code != stdhttp.StatusOK {
		t.Fatalf("expected save restore 200, got %d: %s", restore.Code, restore.Body.String())
	}

	crossInstance := httptest.NewRecorder()
	router.ServeHTTP(crossInstance, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/minecraft-saves/saves/unknown-id/download", nil))
	if crossInstance.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected missing save 404, got %d", crossInstance.Code)
	}
}

func TestDownloadBackupPrunesMissingFileRecord(t *testing.T) {
	router, db, _ := newTestRouter(t)
	backup := domain.Backup{
		ID:         "missing-backup",
		InstanceID: "backup-source",
		FileName:   "missing.zip",
		WorldName:  "Missing",
		SizeBytes:  5,
		Type:       "Manual",
		CreatedAt:  time.Now(),
	}
	if err := db.CreateBackup(context.Background(), &backup); err != nil {
		t.Fatal(err)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/backups/missing-backup/download", nil))
	if download.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected missing backup download 404, got %d: %s", download.Code, download.Body.String())
	}
	if !strings.Contains(download.Body.String(), "backup file not found on disk") {
		t.Fatalf("expected JSON missing file error, got %q", download.Body.String())
	}
	if _, err := db.GetBackup(context.Background(), backup.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing backup record deleted after download miss, got err=%v", err)
	}
}

func TestRestoreBackupSynchronizesServerMetadataFromRestoredConfig(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("restore-sync", cfg.DataDir)
	server.Name = "Restore Sync"
	server.WorldName = "BackedUpWorld"
	server.Config.WorldName = "BackedUpWorld"
	server.Config.MaxPlayers = 14
	server.Config.Port = 17777
	server.MaxPlayers = 14
	server.Port = 17777
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configText, err := terraria.RenderServerConfig(server.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}
	createTestServer(t, db, server)

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/restore-sync/backups", nil))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected backup create 201, got %d: %s", create.Code, create.Body.String())
	}
	var backup domain.Backup
	if err := json.Unmarshal(create.Body.Bytes(), &backup); err != nil {
		t.Fatal(err)
	}

	stale := server
	stale.WorldName = "StaleWorld"
	stale.Config.WorldName = "StaleWorld"
	stale.Config.MaxPlayers = 4
	stale.Config.Port = 18888
	stale.MaxPlayers = 4
	stale.Port = 18888
	saveTestServer(t, db, stale)
	staleConfig, err := terraria.RenderServerConfig(stale.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte(staleConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	restore := httptest.NewRecorder()
	router.ServeHTTP(restore, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/"+backup.ID+"/restore", nil))
	if restore.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup restore 200, got %d: %s", restore.Code, restore.Body.String())
	}
	restored, err := loadTestServer(db, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.WorldName != "BackedUpWorld" || restored.Config.WorldName != "BackedUpWorld" || restored.MaxPlayers != 14 || restored.Port != 7777 {
		t.Fatalf("expected restored server metadata synchronized from backup config, got %+v", restored)
	}
}

func TestDeleteBackupKeepsRecordWhenFileRemovalFails(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	backupDir := filepath.Join(cfg.DataDir, "backups", "backup-source", "blocked.zip")
	if err := os.MkdirAll(filepath.Join(backupDir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	item := domain.Backup{
		ID:         "blocked-backup",
		InstanceID: "backup-source",
		FileName:   "blocked.zip",
		WorldName:  "Blocked",
		SizeBytes:  1,
		Type:       "Manual",
		CreatedAt:  time.Now(),
	}
	if err := db.CreateBackup(context.Background(), &item); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/backups/blocked-backup", nil))
	if remove.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("expected backup delete to fail when file removal fails, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetBackup(context.Background(), "blocked-backup"); err != nil {
		t.Fatalf("expected backup record to remain after failed file removal, got %v", err)
	}
}

func TestBackupSourceMutationsPruneMissingFiles(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("source", cfg.DataDir)
	createTestServer(t, db, source)
	backup := domain.Backup{ID: "missing-backup-source", InstanceID: "source", FileName: "missing.zip", WorldName: "Source World", SizeBytes: 5, Type: "Manual", CreatedAt: time.Now()}
	if err := db.CreateBackup(context.Background(), &backup); err != nil {
		t.Fatal(err)
	}

	restore := httptest.NewRecorder()
	router.ServeHTTP(restore, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/missing-backup-source/restore", nil))
	if restore.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected missing backup restore 404, got %d: %s", restore.Code, restore.Body.String())
	}
	if _, err := db.GetBackup(context.Background(), backup.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing backup record pruned after restore miss, got err=%v", err)
	}
}

func TestBackupPolicyEndpointsAndScheduler(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("auto-backup-server", cfg.DataDir)
	server.Status = domain.StatusRunning
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(server.DataDir, "serverconfig.txt")
	if err := os.WriteFile(configPath, []byte("auto-backup-test"), 0o600); err != nil {
		t.Fatal(err)
	}
	createTestServer(t, db, server)

	// GET default policy
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/auto-backup-server/backup-policy", nil))
	if getRec.Code != stdhttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	var policy domain.BackupPolicy
	if err := json.Unmarshal(getRec.Body.Bytes(), &policy); err != nil {
		t.Fatal(err)
	}
	if policy.IntervalHours != 6 || policy.RetentionCount != 7 {
		t.Fatalf("expected default 6h and 7 retention, got %+v", policy)
	}

	// PUT invalid policy
	badPutRec := httptest.NewRecorder()
	badBody := `{"enabled": true, "intervalHours": 0, "retentionCount": 5}`
	router.ServeHTTP(badPutRec, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/auto-backup-server/backup-policy", strings.NewReader(badBody)))
	if badPutRec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected 400 for bad interval, got %d", badPutRec.Code)
	}

	// PUT valid policy
	putRec := httptest.NewRecorder()
	goodBody := `{"enabled": true, "intervalHours": 2, "retentionCount": 2}`
	router.ServeHTTP(putRec, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/auto-backup-server/backup-policy", strings.NewReader(goodBody)))
	if putRec.Code != stdhttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}
	if err := json.Unmarshal(putRec.Body.Bytes(), &policy); err != nil {
		t.Fatal(err)
	}
	if !policy.Enabled || policy.IntervalHours != 2 || policy.RetentionCount != 2 || policy.NextRunAt == nil {
		t.Fatalf("expected enabled 2h / 2 retention with nextRunAt, got %+v", policy)
	}

	// Verify server spec in DB
	updatedServer, err := db.GetGameServer(context.Background(), "auto-backup-server")
	if err != nil {
		t.Fatal(err)
	}
	if !updatedServer.Spec.BackupPolicy.Enabled {
		t.Fatalf("expected server spec policy enabled, got %+v", updatedServer.Spec.BackupPolicy)
	}

	// Test retention pruning: 3 auto backups and 1 manual backup
	b1 := domain.Backup{ID: "auto-1", InstanceID: "auto-backup-server", FileName: "auto-1.zip", Type: "Auto", CreatedAt: time.Now().Add(-3 * time.Hour)}
	b2 := domain.Backup{ID: "auto-2", InstanceID: "auto-backup-server", FileName: "auto-2.zip", Type: "Auto", CreatedAt: time.Now().Add(-2 * time.Hour)}
	b3 := domain.Backup{ID: "auto-3", InstanceID: "auto-backup-server", FileName: "auto-3.zip", Type: "Auto", CreatedAt: time.Now().Add(-1 * time.Hour)}
	manual := domain.Backup{ID: "manual-1", InstanceID: "auto-backup-server", FileName: "manual-1.zip", Type: "Manual", CreatedAt: time.Now().Add(-4 * time.Hour)}
	for _, b := range []*domain.Backup{&b1, &b2, &b3, &manual} {
		if err := db.CreateBackup(context.Background(), b); err != nil {
			t.Fatal(err)
		}
	}

	h := &Handler{store: db, cfg: cfg}
	h.pruneScheduledBackups(context.Background(), "auto-backup-server", 2)

	// Oldest auto-1 should be pruned, auto-2 and auto-3 remain
	if _, err := db.GetBackup(context.Background(), "auto-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected auto-1 to be pruned, got err=%v", err)
	}
	if _, err := db.GetBackup(context.Background(), "auto-2"); err != nil {
		t.Fatalf("expected auto-2 to remain, got err=%v", err)
	}
	if _, err := db.GetBackup(context.Background(), "auto-3"); err != nil {
		t.Fatalf("expected auto-3 to remain, got err=%v", err)
	}
	// Manual backup should never be pruned
	if _, err := db.GetBackup(context.Background(), "manual-1"); err != nil {
		t.Fatalf("expected manual backup to never be pruned, got err=%v", err)
	}
}
