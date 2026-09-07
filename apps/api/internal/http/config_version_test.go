package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestIncompatibleConfigMutationsPreserveServerAndBackup(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	createTestServer(t, db, testServer("versioned", cfg.DataDir))
	ctx := context.Background()
	server, err := db.GetGameServer(ctx, "versioned")
	if err != nil {
		t.Fatal(err)
	}
	server.Spec.ConfigVersion = 99
	if err := db.SaveGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(server.Spec)
	backup := domain.Backup{ID: "version-backup", InstanceID: server.ID, FileName: "missing.zip"}
	if err := db.CreateBackup(ctx, &backup); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/servers/versioned/config", `{"config":{}}`},
		{http.MethodPost, "/api/backups/version-backup/restore", ``},
		{http.MethodPost, "/api/servers/versioned/saves/version-backup/restore", ``},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(request.method, request.path, strings.NewReader(request.body)))
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "config version") {
			t.Fatalf("%s: %d %s", request.path, response.Code, response.Body.String())
		}
	}
	stored, err := db.GetGameServer(ctx, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(stored.Spec)
	if string(before) != string(after) {
		t.Fatal("config changed despite failed compatibility check")
	}
	if _, err := db.GetBackup(ctx, backup.ID); err != nil {
		t.Fatalf("backup pruned before preflight: %v", err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/terraria/config/preview", strings.NewReader(`{"configVersion":99,"config":{}}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("preview: %d %s", response.Code, response.Body.String())
	}
}

func TestBackupSourceCompatibilityIsCheckedBeforeArchiveAccess(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	createTestServer(t, db, testServer("backup-target", cfg.DataDir))
	ctx := context.Background()
	for _, source := range []domain.Backup{
		{ID: "future", InstanceID: "backup-target", ProviderKey: domain.ProviderTerrariaVanilla, ConfigVersion: 2, FileName: "future.zip"},
		{ID: "foreign", InstanceID: "backup-target", ProviderKey: domain.ProviderPalworld, ConfigVersion: 1, FileName: "foreign.zip"},
	} {
		if err := db.CreateBackup(ctx, &source); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/api/backups/" + source.ID + "/restore", "/api/servers/backup-target/saves/" + source.ID + "/restore"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
			if response.Code != http.StatusConflict {
				t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
			}
		}
		stored, err := db.GetBackup(ctx, source.ID)
		if err != nil || stored.ConfigVersion != source.ConfigVersion || stored.ProviderKey != source.ProviderKey {
			t.Fatalf("source metadata lost: %+v %v", stored, err)
		}
	}
}

func TestRestoreChecksEmbeddedVersionEvenWhenRecordMatches(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	fixture := testServer("embedded-target", cfg.DataDir)
	createTestServer(t, db, fixture)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "world"), []byte("future"), 0600); err != nil {
		t.Fatal(err)
	}
	metadata := backupsvc.Metadata{FormatVersion: 1, GameKey: string(fixture.GameKey), ProviderKey: string(fixture.ProviderKey), ConfigVersion: 2}
	path, _, err := backupsvc.NewService(cfg.DataDir).WithMetadata(metadata).Create(fixture.ID, source)
	if err != nil {
		t.Fatal(err)
	}
	item := domain.Backup{ID: "embedded", InstanceID: fixture.ID, FileName: filepath.Base(path), ProviderKey: fixture.ProviderKey, ConfigVersion: 1}
	if err := db.CreateBackup(context.Background(), &item); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/backups/embedded/restore", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "config version") {
		t.Fatalf("restore=%d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(fixture.DataDir, "world")); !os.IsNotExist(err) {
		t.Fatalf("archive extracted before check: %v", err)
	}
}
