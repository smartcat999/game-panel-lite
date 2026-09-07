package docker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type artifactSourceFunc func(context.Context, workload.Assignment, workload.Artifact) (io.ReadCloser, error)

func (f artifactSourceFunc) Open(ctx context.Context, a workload.Assignment, item workload.Artifact) (io.ReadCloser, error) {
	return f(ctx, a, item)
}

type trackedArtifact struct {
	io.Reader
	closed   bool
	closeErr error
	bytes    int
}

func (r *trackedArtifact) Close() error { r.closed = true; return r.closeErr }
func (r *trackedArtifact) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytes += n
	return n, err
}
func TestArtifactPreparationAndRollback(t *testing.T) {
	payload := []byte{0, 255, 128, 1, 2, 3, 254}
	sum := sha256.Sum256(payload)
	for _, mode := range []string{"success", "digest", "short", "long", "read-error", "close-error", "canceled", "commit-error", "uncertain", "symlink", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "mod.bin")
			config := filepath.Join(dir, "settings.ini")
			if err := os.WriteFile(target, []byte("previous"), 0640); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config, []byte("old-config"), 0600); err != nil {
				t.Fatal(err)
			}
			item := workload.Artifact{ID: "library-id", Path: "mod.bin", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(payload))}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := &trackedArtifact{Reader: bytes.NewReader(payload)}
			switch mode {
			case "digest":
				item.SHA256 = strings.Repeat("0", 64)
			case "short":
				stream.Reader = bytes.NewReader(payload[:3])
			case "long":
				stream.Reader = bytes.NewReader(append(append([]byte{}, payload...), 0, 1, 2))
			case "read-error":
				stream.Reader = io.MultiReader(bytes.NewReader(payload[:3]), errorReader{})
			case "close-error":
				stream.closeErr = errors.New("close failed")
			case "canceled":
				cancel()
			case "symlink":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(config, target); err != nil {
					t.Fatal(err)
				}
			case "conflict":
				item.Path = "settings.ini"
			}
			opened := 0
			a := &Adapter{artifactLimits: ArtifactLimits{MaxFiles: 2, MaxFileBytes: 100, MaxTotalBytes: 100}, artifactSource: artifactSourceFunc(func(_ context.Context, assignment workload.Assignment, ref workload.Artifact) (io.ReadCloser, error) {
				opened++
				if assignment.UID != "uid" || ref.ID != item.ID {
					t.Fatal("source lost assignment binding")
				}
				return stream, nil
			})}
			assignment := workload.Assignment{UID: "uid", Spec: workload.Spec{Options: workload.Options{Files: map[string]string{"settings.ini": "new-config"}, Artifacts: []workload.Artifact{item}}}}
			commits := 0
			_, err := prepareFilesWithArtifacts(dir, assignment.Spec.Options, func(w io.Writer, ref workload.Artifact) error { return a.writeArtifact(ctx, assignment, ref, w) }, func([]string) error {
				commits++
				if mode == "commit-error" {
					return errors.New("commit failed")
				}
				if mode == "uncertain" {
					return errCreationUncertain
				}
				return nil
			})
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid preparation succeeded")
			}
			if opened > 0 && !stream.closed {
				t.Fatal("source not closed")
			}
			if mode == "long" && stream.bytes != len(payload)+1 {
				t.Fatalf("over-read: %d", stream.bytes)
			}
			if mode == "symlink" || mode == "conflict" || mode == "canceled" {
				if opened != 0 {
					t.Fatal("opened source before validation")
				}
			}
			if mode != "success" && mode != "commit-error" && mode != "uncertain" && commits != 0 {
				t.Fatal("committed unverified artifact")
			}
			actual, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatal(readErr)
			}
			want := []byte("previous")
			if mode == "success" || mode == "uncertain" {
				want = payload
			}
			if mode == "symlink" {
				want = []byte("old-config")
			}
			if !bytes.Equal(actual, want) {
				t.Fatalf("artifact changed: %v", actual)
			}
			actual, readErr = os.ReadFile(config)
			if readErr != nil {
				t.Fatal(readErr)
			}
			expected := "old-config"
			if mode == "success" || mode == "uncertain" {
				expected = "new-config"
			}
			if string(actual) != expected {
				t.Fatalf("config changed: %s", actual)
			}
			pending, _ := filepath.Glob(filepath.Join(dir, ".gamepanel-prepare-*"))
			if mode == "uncertain" {
				if len(pending) != 1 {
					t.Fatal("recovery missing")
				}
				if _, err := prepareFiles(dir, workload.Options{}); err == nil {
					t.Fatal("retry bypassed recovery")
				}
			} else if len(pending) != 0 {
				t.Fatal("temporary files leaked")
			}
		})
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func TestArtifactLimitsFailBeforeRuntimeIO(t *testing.T) {
	item := workload.Artifact{ID: "id", Path: "mod.bin", SHA256: strings.Repeat("a", 64), SizeBytes: 10}
	assignment := workload.Assignment{Spec: workload.Spec{Options: workload.Options{Artifacts: []workload.Artifact{item}}}}
	a := &Adapter{}
	if err := a.Create(context.Background(), assignment); err == nil {
		t.Fatal("accepted missing delivery configuration")
	}
	a.artifactSource = artifactSourceFunc(func(context.Context, workload.Assignment, workload.Artifact) (io.ReadCloser, error) {
		t.Fatal("opened source while validating limits")
		return nil, nil
	})
	for _, limit := range []ArtifactLimits{{}, {1, 9, 100}, {1, 100, 9}} {
		a.artifactLimits = limit
		if err := a.ValidateArtifacts(assignment); err == nil {
			t.Fatalf("accepted limits %+v", limit)
		}
	}
	a.artifactLimits = ArtifactLimits{1, 10, 10}
	if err := a.ValidateArtifacts(assignment); err != nil {
		t.Fatal(err)
	}
	second := item
	second.Path = "second.bin"
	assignment.Spec.Options.Artifacts = append(assignment.Spec.Options.Artifacts, second)
	if err := a.ValidateArtifacts(assignment); err == nil {
		t.Fatal("accepted excess count")
	}
	a.artifactLimits = ArtifactLimits{2, 10, 19}
	if err := a.ValidateArtifacts(assignment); err == nil {
		t.Fatal("accepted excess total")
	}
}

func TestAdapterCreatesOnlyAfterArtifactVerification(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			payload := []byte{0, 255, 128, 42}
			sum := sha256.Sum256(payload)
			ref := workload.Artifact{ID: "id", Path: "Mods/mod.bin", SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(sum[:])}
			if !valid {
				ref.SHA256 = strings.Repeat("0", 64)
			}
			var adapter *Adapter
			creates := 0
			adapter = testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/json"):
					w.WriteHeader(http.StatusNotFound)
					io.WriteString(w, `{"message":"missing"}`)
				case strings.HasSuffix(r.URL.Path, "/images/create"):
					io.WriteString(w, "{}\n")
				case strings.HasSuffix(r.URL.Path, "/containers/create"):
					creates++
					content, err := os.ReadFile(filepath.Join(adapter.dataDir, "server", "Mods", "mod.bin"))
					if err != nil || !bytes.Equal(content, payload) {
						t.Errorf("container created without verified artifact: %v", err)
					}
					io.WriteString(w, `{"Id":"created"}`)
				default:
					t.Errorf("unexpected Docker call: %s", r.URL.Path)
					w.WriteHeader(500)
				}
			})
			adapter.artifactSource = artifactSourceFunc(func(context.Context, workload.Assignment, workload.Artifact) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(payload)), nil
			})
			adapter.artifactLimits = ArtifactLimits{1, 100, 100}
			err := adapter.Create(context.Background(), workload.Assignment{UID: "uid", NodeID: "node", ServerID: "server", Generation: 1, Spec: workload.Spec{Image: "image", Options: workload.Options{Artifacts: []workload.Artifact{ref}}}})
			if valid {
				if err != nil || creates != 1 {
					t.Fatalf("valid create: %v / %d", err, creates)
				}
			} else if err == nil || creates != 0 {
				t.Fatalf("invalid create: %v / %d", err, creates)
			}
		})
	}
}
