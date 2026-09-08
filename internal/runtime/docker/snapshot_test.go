package docker

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/archive"
	"github.com/smartcat999/game-panel-lite/internal/worker"
)

func TestStoppedDataUsesLifecycleLock(t *testing.T) {
	observed := worker.State{Exists: true, Managed: true, ID: strings.Repeat("a", 64), ServerID: "server", NodeID: "node", UID: "assignment", Generation: 1}
	var starts atomic.Int64
	var invalid atomic.Bool
	var missing atomic.Bool
	adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/start") {
			starts.Add(1)
			w.WriteHeader(204)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/json") {
			t.Error("unexpected request")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var state any = map[string]any{"Status": "exited", "Running": invalid.Load()}
		if missing.Load() {
			state = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": observed.ID, "State": state, "Config": map[string]any{"Labels": map[string]string{labelManaged: "true", labelServer: observed.ServerID, labelNode: observed.NodeID, labelUID: observed.UID, labelGeneration: "1"}}})
	})
	if err := os.Mkdir(filepath.Join(adapter.dataDir, "server"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adapter.dataDir, "server", "world"), []byte("stable world"), 0600); err != nil {
		t.Fatal(err)
	}
	second := *adapter
	err := adapter.ReadStoppedData(context.Background(), observed, func(ctx context.Context, files fs.FS) error {
		data, err := fs.ReadFile(files, "world")
		if err != nil || string(data) != "stable world" {
			t.Fatal("scoped read failed", err)
		}
		if _, err := fs.ReadFile(files, "../outside"); err == nil {
			t.Fatal("escaped instance directory")
		}
		var encoded bytes.Buffer
		if err := archive.Write(ctx, &encoded, files, ".", nil); err != nil {
			return err
		}
		reader, err := zip.NewReader(bytes.NewReader(encoded.Bytes()), int64(encoded.Len()))
		if err != nil || len(reader.File) != 1 || reader.File[0].Name != "world" {
			t.Fatal("stopped data not archived", err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		if err := second.Start(waitCtx, observed); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("start was not excluded: %v", err)
		}
		return nil
	})
	if err != nil || starts.Load() != 0 {
		t.Fatal("snapshot lock failed", err)
	}
	if err := second.Start(context.Background(), observed); err != nil || starts.Load() != 1 {
		t.Fatal("snapshot retained lifecycle lock", err)
	}
	called := false
	invalid.Store(true)
	if err := adapter.ReadStoppedData(context.Background(), observed, func(context.Context, fs.FS) error { called = true; return nil }); !errors.Is(err, ErrSnapshotUnavailable) || called {
		t.Fatal("running container data exposed")
	}
	invalid.Store(false)
	err = adapter.ReadStoppedData(context.Background(), observed, func(context.Context, fs.FS) error { invalid.Store(true); return nil })
	if !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatal("state change during snapshot accepted")
	}
	invalid.Store(false)
	wrong := observed
	wrong.UID = "old"
	if err := adapter.ReadStoppedData(context.Background(), wrong, func(context.Context, fs.FS) error { t.Error("wrong identity exposed"); return nil }); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatal("stale assignment accepted", err)
	}
	missing.Store(true)
	if err := adapter.ReadStoppedData(context.Background(), observed, func(context.Context, fs.FS) error { t.Error("missing stop evidence accepted"); return nil }); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatal("missing Docker state accepted", err)
	}
	missing.Store(false)
	failure := errors.New("archive failed")
	if err := adapter.ReadStoppedData(context.Background(), observed, func(context.Context, fs.FS) error { return failure }); !errors.Is(err, failure) {
		t.Fatal("archive failure lost")
	}
	if err := second.Start(context.Background(), observed); err != nil {
		t.Fatal("failure retained lock", err)
	}
}
