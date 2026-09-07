package docker

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestMutationsWaitForConfigurationCommit(t *testing.T) {
	var calls atomic.Int32
	adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	observed := worker.State{Exists: true, Managed: true, ID: strings.Repeat("a", 64), ServerID: "server", NodeID: "node", UID: "uid", Generation: 1}
	unlock, err := lockInstanceCreation(context.Background(), adapter.dataDir, observed.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	for name, run := range map[string]func(context.Context, worker.State) error{"start": adapter.Start, "stop": adapter.Stop, "remove": adapter.Remove} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
			defer cancel()
			if err := run(ctx, observed); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("mutation bypassed preparation lock: %v", err)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatal("locked mutations reached Docker")
	}
}

func TestPendingRecoveryPreventsStartButAllowsShutdown(t *testing.T) {
	var calls atomic.Int32
	adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.HasSuffix(r.URL.Path, "/start") {
			t.Error("started with pending recovery")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	observed := worker.State{Exists: true, Managed: true, ID: strings.Repeat("b", 64), ServerID: "server", NodeID: "node", UID: "uid", Generation: 1}
	_, err := prepareFilesAndCommit(filepath.Join(adapter.dataDir, "server"), workload.Options{Files: map[string]string{"config.ini": "new"}}, func([]string) error { return errCreationUncertain })
	if !errors.Is(err, errCreationUncertain) {
		t.Fatal(err)
	}
	if err := adapter.Start(context.Background(), observed); err == nil || !strings.Contains(err.Error(), "recovery") {
		t.Fatalf("pending recovery accepted: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("start reached Docker")
	}
	if err := adapter.Stop(context.Background(), observed); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Remove(context.Background(), observed); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("shutdown blocked: %d", calls.Load())
	}
}
