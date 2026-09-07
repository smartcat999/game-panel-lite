package docker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestWorkerPrefetchesArtifactsBeforeReplacingContainer(t *testing.T) {
	for _, mode := range []string{"success", "bad-second-file", "ownership-changed", "already-current"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			payload := []byte{0, 255, 128, 42}
			digest := sha256.Sum256(payload)
			ref := workload.Artifact{ID: "source", Path: "Mods/mod.bin", SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(digest[:])}
			second := ref
			second.ID = "second"
			second.Path = "Mods/second.bin"
			assignment := workload.Assignment{UID: "uid", NodeID: "node", ServerID: "server", Generation: 2, DesiredState: "running", Spec: workload.Spec{Image: "image", Options: workload.Options{Artifacts: []workload.Artifact{ref, second}}}}
			// HTTP fake Docker and the source share state, just as separate processes do.
			var mu sync.Mutex
			state := worker.State{Exists: true, ID: strings.Repeat("a", 64), Running: true, Managed: true, ServerID: "server", NodeID: "node", UID: "uid", Generation: 1}
			if mode == "already-current" {
				state.Generation = 2
			}
			calls := []string{}
			var adapter *Adapter
			adapter = testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/json"):
					if !state.Exists {
						w.WriteHeader(404)
						io.WriteString(w, `{"message":"missing"}`)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"Id": state.ID, "State": map[string]any{"Running": state.Running}, "Config": map[string]any{"Labels": map[string]string{labelManaged: "true", labelServer: state.ServerID, labelNode: state.NodeID, labelUID: state.UID, labelGeneration: fmt.Sprint(state.Generation)}}})
				case r.Method == http.MethodDelete:
					calls = append(calls, "remove")
					state.Exists = false
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/images/create"):
					io.WriteString(w, "{}\n")
				case strings.HasSuffix(r.URL.Path, "/containers/create"):
					for _, item := range []workload.Artifact{ref, second} {
						content, err := os.ReadFile(filepath.Join(adapter.dataDir, "server", filepath.FromSlash(item.Path)))
						if err != nil || !bytes.Equal(content, payload) {
							t.Errorf("unverified file at create: %v", err)
						}
					}
					calls = append(calls, "create")
					state.Exists = true
					state.ID = strings.Repeat("b", 64)
					state.Generation = 2
					state.Running = false
					json.NewEncoder(w).Encode(map[string]string{"Id": state.ID})
				case strings.HasSuffix(r.URL.Path, "/start"):
					calls = append(calls, "start")
					state.Running = true
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			})
			adapter.artifactLimits = ArtifactLimits{2, 100, 200}
			adapter.artifactSource = artifactSourceFunc(func(_ context.Context, a workload.Assignment, item workload.Artifact) (io.ReadCloser, error) {
				mu.Lock()
				defer mu.Unlock()
				calls = append(calls, "fetch-"+item.ID)
				if !state.Exists || state.ID != strings.Repeat("a", 64) {
					t.Error("removed old container before prefetch completed")
				}
				if mode == "ownership-changed" {
					state.UID = "another-assignment"
				}
				content := payload
				if mode == "bad-second-file" && item.ID == "second" {
					content = []byte{1, 2, 3, 4}
				}
				return io.NopCloser(bytes.NewReader(content)), nil
			})
			result := worker.Reconcile(ctx, assignment, adapter)
			mu.Lock()
			defer mu.Unlock()
			switch mode {
			case "success":
				if result.LastError != "" || result.ActualState != "running" || strings.Join(calls, ",") != "fetch-source,fetch-second,remove,create,start" {
					t.Fatalf("reconcile: %+v %v", result, calls)
				}
			case "already-current":
				if result.LastError != "" || len(calls) != 0 {
					t.Fatalf("current workload redownloaded: %+v %v", result, calls)
				}
			default:
				if result.LastError == "" || !state.Exists || state.ID != strings.Repeat("a", 64) || !state.Running || strings.Contains(strings.Join(calls, ","), "remove") {
					t.Fatalf("old workload affected: %+v %v", result, calls)
				}
			}
			pending, err := filepath.Glob(filepath.Join(adapter.dataDir, artifactPreparationDir, "*"))
			if err != nil || len(pending) != 0 {
				t.Fatalf("prefetch leaked: %v %v", pending, err)
			}
		})
	}
}

func TestPreparedArtifactsStayBoundAndRelease(t *testing.T) {
	content := []byte("verified")
	hash := sha256.Sum256(content)
	ref := workload.Artifact{ID: "source", Path: "Mods/source.bin", SizeBytes: int64(len(content)), SHA256: hex.EncodeToString(hash[:])}
	assignment := workload.Assignment{UID: "uid", NodeID: "node", ServerID: "server", Generation: 1, Spec: workload.Spec{Options: workload.Options{Artifacts: []workload.Artifact{ref}}}}
	calls := 0
	a := &Adapter{dataDir: t.TempDir(), artifactLimits: ArtifactLimits{1, 100, 100}, artifactSource: artifactSourceFunc(func(context.Context, workload.Assignment, workload.Artifact) (io.ReadCloser, error) {
		calls++
		return io.NopCloser(bytes.NewReader(content)), nil
	})}
	ready, err := a.PrepareArtifacts(context.Background(), assignment)
	if err != nil {
		t.Fatal(err)
	}
	prepared := ready.(*preparedArtifactRuntime)
	content = []byte("changed!")
	for i := 0; i < 2; i++ {
		var result bytes.Buffer
		if err := prepared.writeArtifact(context.Background(), assignment, ref, &result); err != nil || result.String() != "verified" {
			t.Fatalf("cached bytes: %q %v", result.String(), err)
		}
	}
	if calls != 1 {
		t.Fatalf("source reopened: %d", calls)
	}
	for _, change := range []func(*workload.Assignment){func(a *workload.Assignment) { a.UID = "foreign" }, func(a *workload.Assignment) { a.NodeID = "foreign" }, func(a *workload.Assignment) { a.ServerID = "foreign" }, func(a *workload.Assignment) { a.Generation++ }} {
		foreign := assignment
		change(&foreign)
		if _, err := prepared.cache.Open(context.Background(), foreign, ref); err == nil {
			t.Fatal("foreign assignment read prepared bytes")
		}
	}
	foreign := ref
	foreign.SHA256 = strings.Repeat("0", 64)
	if _, err := prepared.cache.Open(context.Background(), assignment, foreign); err == nil {
		t.Fatal("unprepared descriptor read bytes")
	}
	if err := ready.Release(); err != nil {
		t.Fatal(err)
	}
	if err := ready.Release(); err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	if err := prepared.writeArtifact(context.Background(), assignment, ref, &result); err == nil {
		t.Fatal("released bytes still readable")
	}
	entries, err := os.ReadDir(filepath.Join(a.dataDir, artifactPreparationDir))
	if err != nil || len(entries) != 0 {
		t.Fatalf("cleanup: %v %v", entries, err)
	}
	if _, err := containerName(artifactPreparationDir); err == nil {
		t.Fatal("instance can overwrite preparation root")
	}
}
