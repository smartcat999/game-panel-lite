package docker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestPreparationRestoresFilesAndPermissions(t *testing.T) {
	for _, cause := range []string{"invalid-mount", "late-file-conflict", "commit-failure"} {
		t.Run(cause, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700|os.ModeSticky); err != nil {
				t.Fatal(err)
			}
			original := filepath.Join(root, "a.ini")
			if err := os.WriteFile(original, []byte("original"), 0640); err != nil {
				t.Fatal(err)
			}
			options := workload.Options{Files: map[string]string{"a.ini": "replacement", "nested/new.ini": "new"}}
			var commit func([]string) error
			switch cause {
			case "invalid-mount":
				options.DataMounts = []string{"relative-target"}
			case "late-file-conflict":
				if err := os.Mkdir(filepath.Join(root, "z-blocked"), 0755); err != nil {
					t.Fatal(err)
				}
				options.Files["z-blocked"] = "not a directory"
			case "commit-failure":
				commit = func([]string) error { return errors.New("container rejected") }
			}
			if _, err := prepareFilesAndCommit(root, options, commit); err == nil {
				t.Fatal("expected preparation failure")
			}
			content, err := os.ReadFile(original)
			if err != nil || string(content) != "original" {
				t.Fatalf("original not restored: %q %v", content, err)
			}
			info, err := os.Stat(original)
			if err != nil || info.Mode().Perm() != 0640 {
				t.Fatalf("file mode not restored: %v %v", info, err)
			}
			info, err = os.Stat(root)
			if err != nil || info.Mode().Perm() != 0700 || info.Mode()&os.ModeSticky == 0 {
				t.Fatalf("directory mode not restored: %v %v", info, err)
			}
			if _, err := os.Stat(filepath.Join(root, "nested")); !os.IsNotExist(err) {
				t.Fatalf("new directories retained: %v", err)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".gamepanel-prepare-") {
					t.Fatalf("staging leaked: %s", entry.Name())
				}
			}
		})
	}
}

func TestUncertainCreationRetainsRecoveryAndBlocksBlindRetry(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "settings.ini"), []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	options := workload.Options{Files: map[string]string{"settings.ini": "new"}}
	if _, err := prepareFilesAndCommit(root, options, func([]string) error { return errCreationUncertain }); !errors.Is(err, errCreationUncertain) {
		t.Fatalf("uncertain result lost: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".gamepanel-prepare-*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("missing recovery directory: %v %v", matches, err)
	}
	old, err := os.ReadFile(filepath.Join(matches[0], "0.old"))
	if err != nil || string(old) != "old" {
		t.Fatalf("original backup missing: %q %v", old, err)
	}
	mapping, err := os.ReadFile(filepath.Join(matches[0], "0.path.json"))
	if err != nil || !strings.Contains(string(mapping), "settings.ini") {
		t.Fatalf("recovery path mapping missing: %q %v", mapping, err)
	}
	if _, err := prepareFiles(root, options); err == nil {
		t.Fatal("blind retry overwrote pending recovery")
	}
	actual, err := os.ReadFile(filepath.Join(root, "settings.ini"))
	if err != nil || string(actual) != "new" {
		t.Fatalf("uncertain configuration rolled back: %q %v", actual, err)
	}
}

func TestDockerCreateFailureConfirmsBeforeRestoringFiles(t *testing.T) {
	for _, result := range []string{"missing", "created", "unknown"} {
		t.Run(result, func(t *testing.T) {
			var attempted atomic.Bool
			adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/json"):
					if !attempted.Load() || result == "missing" {
						w.WriteHeader(404)
						io.WriteString(w, `{"message":"missing"}`)
						return
					}
					if result == "unknown" {
						w.WriteHeader(503)
						io.WriteString(w, `{"message":"unavailable"}`)
						return
					}
					io.WriteString(w, `{"Id":"`+strings.Repeat("d", 64)+`","Config":{"Labels":{"io.gamepanel.managed":"true","io.gamepanel.server-id":"server","io.gamepanel.node-id":"node","io.gamepanel.assignment-uid":"uid","io.gamepanel.generation":"1"}}}`)
				case strings.HasSuffix(r.URL.Path, "/images/create"):
					io.WriteString(w, `{"status":"ready"}`)
				case strings.HasSuffix(r.URL.Path, "/containers/create"):
					attempted.Store(true)
					w.WriteHeader(500)
					io.WriteString(w, `{"message":"creation response failed"}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(500)
				}
			})
			path := filepath.Join(adapter.dataDir, "server", "config.ini")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("old"), 0640); err != nil {
				t.Fatal(err)
			}
			err := adapter.Create(context.Background(), workload.Assignment{UID: "uid", ServerID: "server", NodeID: "node", Generation: 1, Spec: workload.Spec{Image: "example:1", Options: workload.Options{Files: map[string]string{"config.ini": "new"}}}})
			if (err == nil) != (result == "created") {
				t.Fatalf("unexpected creation result: %v", err)
			}
			if result == "unknown" && !errors.Is(err, errCreationUncertain) {
				t.Fatalf("uncertainty lost: %v", err)
			}
			want := "new"
			if result == "missing" {
				want = "old"
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil || string(content) != want {
				t.Fatalf("configuration %q want %q: %v", content, want, readErr)
			}
		})
	}
}
