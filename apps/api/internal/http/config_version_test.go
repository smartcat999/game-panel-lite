package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
