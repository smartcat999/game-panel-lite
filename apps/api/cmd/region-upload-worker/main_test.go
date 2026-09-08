package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assetfiles"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestRegionalUploadEntry(t *testing.T) {
	dsn := os.Getenv("GAMEPANEL_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "upload_cli_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	if err := store.MigrateRegionalPostgres(ctx, u.String(), "east"); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenRegionalPostgres(u.String(), "east", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	data := []byte("prepared archive fixture")
	version := assets.PublishedVersion{OrganizationID: "tenant", AssetID: "archive", Version: "v1", SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), SizeBytes: int64(len(data))}
	request := backup.Requested{SchemaVersion: 1, EventID: "request", OperationID: "operation", BackupID: "backup", OrganizationID: "tenant", ServerID: "server", RegionID: "east", RevisionID: "revision", SpecGeneration: 1, IntentVersion: 1, PlacementEpoch: 1, Scope: "world"}
	if err := db.RecordBackupRequest(ctx, request); err != nil {
		t.Fatal(err)
	}
	plan := backup.UploadPlan{ID: "upload", OperationID: request.OperationID, RequestEventID: request.EventID, RegionID: "east", ServerID: "server", DeploymentID: "deployment", NodeID: "node", SnapshotID: "snapshot", PlacementEpoch: 1, StorageID: "storage", ObjectKey: "object", Asset: version}
	if err := db.PrepareArchiveUpload(ctx, plan); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files, err := assetfiles.New(dir, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := files.Put(ctx, version, io.NopCloser(bytes.NewReader(data))); err != nil {
		t.Fatal(err)
	}
	if err := files.Close(); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var content []byte
	puts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/archives/object" || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("unexpected S3 request")
			w.WriteHeader(400)
			return
		}
		switch r.Method {
		case "GET":
			if content == nil {
				w.WriteHeader(404)
				return
			}
			w.Header().Set("X-Amz-Version-Id", "v-backend")
			_, _ = w.Write(content)
		case "PUT":
			if content != nil {
				w.WriteHeader(412)
				return
			}
			if r.Header.Get("If-None-Match") != "*" {
				t.Error("missing conditional write")
				w.WriteHeader(400)
				return
			}
			var err error
			content, err = io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			puts++
			w.Header().Set("X-Amz-Version-Id", "v-backend")
			w.WriteHeader(200)
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	o := options{region: "east", dsn: u.String(), directory: dir, storageID: "storage", endpoint: server.URL, signingRegion: "test", bucket: "archives", ca: ca, accessKey: "test-access", secretKey: "test-secret", lease: 5 * time.Second, transferTimeout: time.Second, taskTimeout: 2 * time.Second, retry: time.Second, poll: 10 * time.Millisecond, maxBytes: 1024}
	workCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- run(workCtx, o) }()
	for {
		var status string
		if err := admin.QueryRowContext(ctx, "SELECT status FROM "+schema+".regional_backup_requests WHERE operation_id=$1", request.OperationID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "uploaded" {
			break
		}
		select {
		case err := <-done:
			t.Fatal("worker stopped early", err)
		case <-ctx.Done():
			t.Fatal("upload did not complete")
		case <-time.After(10 * time.Millisecond):
		}
	}
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("worker did not stop")
	}
	var payload string
	if err := admin.QueryRowContext(ctx, "SELECT payload FROM "+schema+".regional_backup_result_outbox WHERE operation_id=$1", request.OperationID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event backup.ArchiveUploaded
	if json.Unmarshal([]byte(payload), &event) != nil || event.Validate() != nil || event.Plan != plan || event.Receipt.ObjectVersion != "v-backend" {
		t.Fatal("wrong result outbox")
	}
	mu.Lock()
	defer mu.Unlock()
	if puts != 1 || !bytes.Equal(content, data) {
		t.Fatal("wrong uploaded bytes")
	}
}

func TestRejectInvalidUploadSettings(t *testing.T) {
	for _, o := range []options{{}, {region: "east", dsn: "private database URL", accessKey: "secret", secretKey: "secret"}} {
		if err := run(context.Background(), o); err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid settings not safely rejected", err)
		}
	}
}
