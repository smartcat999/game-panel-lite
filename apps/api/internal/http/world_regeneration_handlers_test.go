package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestWorldRegenerationCapabilityIsProviderScoped(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	terrariaServer := testServer("world-regeneration-unsupported", cfg.DataDir)
	createTestServer(t, db, terrariaServer)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+terrariaServer.ID+"/world-regeneration", nil))
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	var view worldRegenerationView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Supported {
		t.Fatal("expected Terraria world regeneration to remain hidden")
	}
}

func TestDSTWorldRegenerationBacksUpAndRemovesShardSaves(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	payload := dst.NewProvider().DefaultConfigPayload()
	identity := payload["identity"].(map[string]any)
	identity["clusterToken"] = "test-cluster-token"
	identity["clusterName"] = "FriendsCluster"
	payload["caves"] = map[string]any{"enabled": true, "preset": "cave_default", "overrides": map[string]any{}}
	server := testServerFixture{
		ID: "dst-regenerate", Name: "DST Friends", GameKey: domain.GameDST, ProviderKey: domain.ProviderDST,
		Status: domain.StatusStopped, Port: 10999, HostPort: 10999,
		DataDir: filepath.Join(cfg.DataDir, "instances", "dst-regenerate"), ConfigPayload: payload,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	for _, shard := range []string{"Master", "Caves"} {
		saveDir := filepath.Join(server.DataDir, "dst", "FriendsCluster", shard, "save")
		if err := os.MkdirAll(saveDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(saveDir, "session"), []byte("old-"+shard), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	createTestServer(t, db, server)

	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/world-regeneration", nil))
	if status.Code != stdhttp.StatusOK || !strings.Contains(status.Body.String(), `"supported":true`) {
		t.Fatalf("expected supported status, got %d: %s", status.Code, status.Body.String())
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/world-regeneration", strings.NewReader(`{"startAfter":false}`)))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected regeneration 202, got %d: %s", start.Code, start.Body.String())
	}
	job := waitForWorldRegenerationJob(t, db, server.ID)
	if job.Status != domain.WorldRegenerationJobSucceeded || job.BackupID == "" {
		t.Fatalf("expected successful regeneration with backup, got %+v", job)
	}
	for _, shard := range []string{"Master", "Caves"} {
		if _, err := os.Stat(filepath.Join(server.DataDir, "dst", "FriendsCluster", shard, "save")); !os.IsNotExist(err) {
			t.Fatalf("expected %s save removed until next start, err=%v", shard, err)
		}
	}
	backup, err := db.GetBackup(context.Background(), job.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Type != "Pre-regeneration" {
		t.Fatalf("expected pre-regeneration backup, got %+v", backup)
	}
}

func waitForWorldRegenerationJob(t *testing.T, db *store.Store, instanceID string) domain.WorldRegenerationJob {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job, err := db.GetLatestWorldRegenerationJobByInstance(context.Background(), instanceID)
		if err == nil && (job.Status == domain.WorldRegenerationJobSucceeded || job.Status == domain.WorldRegenerationJobFailed) {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, err := db.GetLatestWorldRegenerationJobByInstance(context.Background(), instanceID)
	t.Fatalf("timed out waiting for regeneration job, job=%+v err=%v", job, err)
	return domain.WorldRegenerationJob{}
}
