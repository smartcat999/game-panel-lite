package http

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type backupSaveAdapter struct {
	availableMockAdapter
	commands chan string
	reader   *io.PipeReader
	writer   *io.PipeWriter
	sendErr  error
}

func newBackupSaveAdapter(t *testing.T) *backupSaveAdapter {
	a := &backupSaveAdapter{availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}, commands: make(chan string, 4)}
	a.reader, a.writer = io.Pipe()
	t.Cleanup(func() { a.reader.Close(); a.writer.Close() })
	return a
}
func (a *backupSaveAdapter) LogsWorkload(context.Context, string, bool) (io.ReadCloser, error) {
	return a.reader, nil
}
func (a *backupSaveAdapter) SendCommandWorkload(_ context.Context, _ string, command string) error {
	if a.sendErr != nil {
		return a.sendErr
	}
	a.commands <- command
	return nil
}

func dstBackupFixture(t *testing.T) (*Handler, domain.GameServer, *backupSaveAdapter, string) {
	t.Helper()
	root := t.TempDir()
	a := newBackupSaveAdapter(t)
	p := dst.NewProvider()
	payload := p.DefaultConfigPayload()
	payload["identity"] = map[string]any{"clusterName": "Cluster"}
	payload["caves"] = map[string]any{"enabled": true}
	server := domain.GameServer{ID: "dst-backup", ProviderKey: domain.ProviderDST, Spec: domain.ServerSpec{Config: payload, Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(root, "instance")}}, Status: domain.ServerRuntimeStatus{RuntimeID: "runtime", Phase: domain.PhaseRunning, ActualState: domain.ActualRunning}}
	for _, shard := range []string{"Master", "Caves"} {
		p := filepath.Join(server.Spec.Runtime.DataDir, "dst/Cluster", shard, "save/world")
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte("old"), 0600)
	}
	h := &Handler{cfg: config.Config{DataDir: root}, provider: provider.NewRegistry(p), runtime: runtime.NewSwitchableAdapter(a)}
	return h, server, a, root
}

var backupAckPattern = regexp.MustCompile(`GAMEPANEL_SAVE_[a-z0-9-]+_(?:master|caves)`)

func receiveBackupCommand(t *testing.T, a *backupSaveAdapter) (string, string) {
	t.Helper()
	select {
	case cmd := <-a.commands:
		parts := strings.SplitN(cmd, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("not shard routed: %q", cmd)
		}
		lua, err := base64.StdEncoding.DecodeString(parts[2])
		if err != nil {
			t.Fatal(err)
		}
		marker := backupAckPattern.FindString(string(lua))
		if marker == "" {
			t.Fatal("missing completion marker")
		}
		return parts[1], marker
	case <-time.After(2 * time.Second):
		t.Fatal("save command not sent")
		return "", ""
	}
}
func emitBackupLog(t *testing.T, a *backupSaveAdapter, line string) {
	t.Helper()
	if _, err := io.WriteString(a.writer, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

func TestDSTBackupWaitsForBothNativeSaveCallbacksBeforeArchiving(t *testing.T) {
	h, s, a, root := dstBackupFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type result struct {
		path string
		err  error
	}
	done := make(chan result, 1)
	go func() { p, _, e := h.createInstanceBackupArchive(ctx, s, s.Spec.Runtime.DataDir); done <- result{p, e} }()
	master, ackMaster := receiveBackupCommand(t, a)
	caves, ackCaves := receiveBackupCommand(t, a)
	if master != "master" || caves != "caves" {
		t.Fatal("missing shard")
	}
	emitBackupLog(t, a, "[00:00:00]: GAMEPANEL_SAVE_old-backup_master")
	for _, shard := range []string{"Master", "Caves"} {
		os.WriteFile(filepath.Join(s.Spec.Runtime.DataDir, "dst/Cluster", shard, "save/world"), []byte("saved-"+shard), 0600)
	}
	emitBackupLog(t, a, "[00:00:01]: "+ackMaster)
	if entries, _ := os.ReadDir(filepath.Join(root, "backups", s.ID)); len(entries) != 0 {
		t.Fatal("archive created before caves save completed")
	}
	select {
	case r := <-done:
		t.Fatalf("completed without caves confirmation: %v", r.err)
	default:
	}
	emitBackupLog(t, a, "[00:00:02]: "+ackCaves)
	r := <-done
	if r.err != nil {
		t.Fatal(r.err)
	}
	z, err := zip.OpenReader(r.path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != 2 {
		t.Fatalf("unexpected native files: %d", len(z.File))
	}
	for _, f := range z.File {
		rd, _ := f.Open()
		b, _ := io.ReadAll(rd)
		rd.Close()
		if !strings.HasPrefix(f.Name, "Cluster/") || !strings.HasPrefix(string(b), "saved-") {
			t.Fatalf("wrong layout or stale save: %s %s", f.Name, b)
		}
	}
}

func TestDSTBackupDoesNotCreateZIPOnMissingFailedOrCancelledSave(t *testing.T) {
	for _, mode := range []string{"timeout", "stream-ended", "failed", "send-error"} {
		t.Run(mode, func(t *testing.T) {
			h, s, a, root := dstBackupFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if mode == "send-error" {
				a.sendErr = errors.New("runtime unavailable")
			}
			done := make(chan error, 1)
			go func() { _, _, err := h.createInstanceBackupArchive(ctx, s, s.Spec.Runtime.DataDir); done <- err }()
			if mode != "send-error" {
				_, master := receiveBackupCommand(t, a)
				_, caves := receiveBackupCommand(t, a)
				emitBackupLog(t, a, "[00:00:01]: "+master)
				if mode == "stream-ended" {
					a.writer.Close()
				}
				if mode == "failed" {
					emitBackupLog(t, a, strings.Replace(caves, "GAMEPANEL_SAVE_", "GAMEPANEL_SAVE_FAILED_", 1))
				}
			}
			if err := <-done; err == nil {
				t.Fatal("unconfirmed save was accepted")
			}
			if entries, _ := os.ReadDir(filepath.Join(root, "backups", s.ID)); len(entries) != 0 {
				t.Fatal("failed save left an archive")
			}
		})
	}
}

func TestStoppedDSTBackupNeedsNoRuntimeAndHTTPRestoresNativeZIP(t *testing.T) {
	for _, endpoint := range []string{"backups", "saves/snapshot"} {
		t.Run(endpoint, func(t *testing.T) {
			router, db, cfg := newTestRouter(t)
			s := testServer("dst-native", cfg.DataDir)
			s.ProviderKey = domain.ProviderDST
			s.GameKey = domain.GameDST
			s.ConfigPayload = dst.NewProvider().DefaultConfigPayload()
			s.ConfigPayload["identity"] = map[string]any{"clusterName": "Cluster"}
			p := filepath.Join(s.DataDir, "dst/Cluster/Master/save/world")
			os.MkdirAll(filepath.Dir(p), 0755)
			os.WriteFile(p, []byte("original"), 0600)
			createTestServer(t, db, s)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+s.ID+"/"+endpoint, nil))
			if rr.Code != stdhttp.StatusCreated {
				t.Fatalf("backup failed: %d %s", rr.Code, rr.Body)
			}
			backups, err := db.ListBackups(context.Background())
			if err != nil || len(backups) != 1 {
				t.Fatalf("backup not registered: %v", err)
			}
			os.WriteFile(p, []byte("changed"), 0600)
			stale := filepath.Join(filepath.Dir(p), "later-snapshot")
			os.WriteFile(stale, []byte("later"), 0600)
			rr = httptest.NewRecorder()
			router.ServeHTTP(rr, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/"+backups[0].ID+"/restore", nil))
			if rr.Code != stdhttp.StatusOK {
				t.Fatalf("restore failed: %d %s", rr.Code, rr.Body)
			}
			if b, _ := os.ReadFile(p); string(b) != "original" {
				t.Fatal("native ZIP restored in wrong directory")
			}
			if _, err := os.Stat(stale); !os.IsNotExist(err) {
				t.Fatal("restore left a later snapshot")
			}
		})
	}
}

func TestScheduledDSTBackupConfirmsSavesBeforeRegisteringAutoBackup(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("save-failed-", fail), func(t *testing.T) {
			h, s, a, root := dstBackupFixture(t)
			db, err := store.Open(filepath.Join(root, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			h.store = db
			h.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			s.Spec.BackupPolicy.Enabled = true
			if err := db.CreateGameServer(context.Background(), &s); err != nil {
				t.Fatal(err)
			}
			if fail {
				a.sendErr = errors.New("console disconnected")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- h.executeScheduledBackup(ctx, s, time.Now(), time.Hour, 3) }()
			if !fail {
				_, master := receiveBackupCommand(t, a)
				_, caves := receiveBackupCommand(t, a)
				emitBackupLog(t, a, master)
				emitBackupLog(t, a, caves)
			}
			err = <-done
			if (err != nil) != fail {
				t.Fatalf("unexpected scheduled result: %v", err)
			}
			backups, err := db.ListBackups(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if fail && len(backups) != 0 {
				t.Fatal("failed scheduled save was registered as a successful backup")
			}
			if !fail && (len(backups) != 1 || backups[0].Type != "Auto") {
				t.Fatalf("expected confirmed automatic backup, got %+v", backups)
			}
		})
	}
}

func TestDSTBackupRejectsTransitionalAndUnknownRuntimeState(t *testing.T) {
	for _, status := range []domain.ServerRuntimeStatus{
		{Phase: domain.PhaseReconciling, ActualState: domain.ActualRunning},
		{Phase: domain.PhaseFailed, ActualState: domain.ActualUnknown},
	} {
		h, s, _, root := dstBackupFixture(t)
		s.Status = status
		if _, _, err := h.createInstanceBackupArchive(context.Background(), s, s.Spec.Runtime.DataDir); err == nil {
			t.Fatal("unconfirmed runtime state accepted")
		}
		if entries, _ := os.ReadDir(filepath.Join(root, "backups", s.ID)); len(entries) != 0 {
			t.Fatal("unconfirmed state produced a backup")
		}
	}
}
