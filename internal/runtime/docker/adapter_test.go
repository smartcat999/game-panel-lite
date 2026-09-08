package docker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func testAdapter(t *testing.T, handle http.HandlerFunc) *Adapter {
	t.Helper()
	server := httptest.NewServer(handle)
	t.Cleanup(server.Close)
	cli, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.44"), client.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return &Adapter{client: cli, dataDir: t.TempDir()}
}
func TestCreatePreservesNetworkResourcesAndOwnership(t *testing.T) {
	var got struct {
		container.Config
		HostConfig container.HostConfig
	}
	adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/json"):
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"message":"missing"}`)
		case strings.HasSuffix(r.URL.Path, "/images/create"):
			io.WriteString(w, `{"status":"ready"}`)
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"Id":"runtime"}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	a := workload.Assignment{UID: "uid", ServerID: "server", NodeID: "node", Generation: 3, Spec: workload.Spec{
		Image: "example:1", DataDir: "/must/not/use/control-plane/path",
		Resources: workload.Resources{CPULimitCores: 1.5, MemoryLimitMB: 2048},
		Network:   workload.Network{Port: 7777, HostPort: 47777, Protocol: "tcp", AdditionalPorts: []workload.Port{{Port: 8888, HostPort: 48888, Protocol: "udp"}}},
		Options:   workload.Options{Files: map[string]string{"settings/server.ini": "content"}},
	}}
	if err := adapter.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if got.HostConfig.NanoCPUs != 1500000000 || got.HostConfig.Memory != 2048*1024*1024 {
		t.Fatalf("resource limits lost: %+v", got.HostConfig.Resources)
	}
	if got.HostConfig.NetworkMode == "host" || got.HostConfig.PortBindings["7777/tcp"][0].HostPort != "47777" || got.HostConfig.PortBindings["8888/udp"][0].HostPort != "48888" {
		t.Fatalf("network lost: %+v", got.HostConfig)
	}
	if got.Labels[labelUID] != "uid" || got.Labels[labelNode] != "node" || got.Labels[labelGeneration] != "3" {
		t.Fatalf("ownership lost: %v", got.Labels)
	}
	if len(got.HostConfig.SecurityOpt) != 1 || got.HostConfig.SecurityOpt[0] != "no-new-privileges:true" {
		t.Fatalf("security opt lost: %+v", got.HostConfig.SecurityOpt)
	}
	expectedCapDrop := []string{"SYS_ADMIN", "NET_ADMIN", "SYS_RAWIO", "SYS_MODULE", "SYS_PTRACE", "SYS_BOOT"}
	if !reflect.DeepEqual([]string(got.HostConfig.CapDrop), expectedCapDrop) {
		t.Fatalf("cap drop mismatch: got %v, want %v", got.HostConfig.CapDrop, expectedCapDrop)
	}
	if len(got.HostConfig.Binds) != 1 || !strings.HasPrefix(got.HostConfig.Binds[0], adapter.dataDir) {
		t.Fatalf("unexpected binds: %v", got.HostConfig.Binds)
	}
	content, err := os.ReadFile(filepath.Join(adapter.dataDir, "server/settings/server.ini"))
	if err != nil || string(content) != "content" {
		t.Fatalf("file=%q err=%v", content, err)
	}
}
func TestPullFailurePreventsContainerCreation(t *testing.T) {
	adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/json") {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"message":"missing"}`)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/images/create") {
			t.Fatalf("unexpected create after failed pull: %s", r.URL.Path)
		}
		io.WriteString(w, `{"error":"registry unavailable"}`)
	})
	err := adapter.Create(context.Background(), workload.Assignment{ServerID: "server", Spec: workload.Spec{Image: "example:1"}})
	if err == nil || !strings.Contains(err.Error(), "registry unavailable") {
		t.Fatalf("pull failure ignored: %v", err)
	}
}
func TestPrepareFilesRejectsTraversalAndEscapingSymlink(t *testing.T) {
	for _, name := range []string{"../escape", "link/escape"} {
		dir, outside := t.TempDir(), t.TempDir()
		if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareFiles(dir, workload.Options{Files: map[string]string{name: "bad"}}); err == nil {
			t.Fatalf("accepted %s", name)
		}
		if _, err := os.Stat(filepath.Join(outside, "escape")); !os.IsNotExist(err) {
			t.Fatalf("outside file touched: %v", err)
		}
	}
}
func TestPrepareFilesRejectsSymlinkMount(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareFiles(dir, workload.Options{DataMounts: []string{"linked:/data"}}); err == nil {
		t.Fatal("accepted symlink mount")
	}
}
func TestLifecycleOperationsAreIdempotent(t *testing.T) {
	adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/start") || strings.HasSuffix(r.URL.Path, "/stop") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message":"missing"}`)
	})
	for _, run := range []func(context.Context, worker.State) error{adapter.Start, adapter.Stop, adapter.Remove} {
		if err := run(context.Background(), worker.State{ID: strings.Repeat("a", 64), Exists: true, Managed: true, ServerID: "server", NodeID: "node", UID: "uid", Generation: 1}); err != nil {
			t.Fatal(err)
		}
	}
	state, err := adapter.Inspect(context.Background(), "server")
	if err != nil || state.Exists {
		t.Fatalf("inspect missing: %+v %v", state, err)
	}
}

func TestMutationsAddressOnlyTheObservedContainerID(t *testing.T) {
	id := strings.Repeat("b", 64)
	calls := 0
	adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.HasPrefix(r.URL.Path, "/v1.44/containers/"+id) {
			t.Errorf("mutation used a reusable target: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message":"observed container was replaced"}`)
	})
	observed := worker.State{ID: id, Exists: true, Managed: true, ServerID: "server", NodeID: "node", UID: "uid", Generation: 1}
	if err := adapter.Start(context.Background(), observed); err == nil {
		t.Fatal("missing start target should fail")
	}
	if err := adapter.Stop(context.Background(), observed); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Remove(context.Background(), observed); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("unexpected calls: %d", calls)
	}
	for _, invalid := range []string{"", "gamepanel-server", id[:12], strings.Repeat("z", 64)} {
		observed.ID = invalid
		for _, run := range []func(context.Context, worker.State) error{adapter.Start, adapter.Stop, adapter.Remove} {
			if err := run(context.Background(), observed); err == nil {
				t.Fatalf("accepted mutable or invalid target %q", invalid)
			}
		}
	}
	if calls != 3 {
		t.Fatal("invalid target reached Docker")
	}
}

func TestCompetingCreateCannotRewriteInstanceFiles(t *testing.T) {
	pullStarted := make(chan struct{})
	releasePull := make(chan struct{})
	var exists atomic.Bool
	var creates atomic.Int32
	adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/json"):
			if exists.Load() {
				io.WriteString(w, `{"Id":"`+strings.Repeat("c", 64)+`"}`)
			} else {
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, `{"message":"missing"}`)
			}
		case strings.HasSuffix(r.URL.Path, "/images/create"):
			close(pullStarted)
			<-releasePull
			io.WriteString(w, `{"status":"ready"}`)
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			creates.Add(1)
			exists.Store(true)
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"Id":"`+strings.Repeat("c", 64)+`"}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	file := filepath.Join(adapter.dataDir, "server", "settings.ini")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("initial"), 0600); err != nil {
		t.Fatal(err)
	}
	first := workload.Assignment{ServerID: "server", Spec: workload.Spec{Image: "example:1", Options: workload.Options{Files: map[string]string{"settings.ini": "first"}}}}
	result := make(chan error, 1)
	go func() { result <- adapter.Create(context.Background(), first) }()
	<-pullStarted
	// Always release the fake pull, including when an assertion fails.
	released := false
	defer func() {
		if !released {
			close(releasePull)
			<-result
		}
	}()
	second := &Adapter{client: adapter.client, dataDir: adapter.dataDir}
	competing := first
	competing.Spec.Options.Files = map[string]string{"settings.ini": "second"}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	err := second.Create(ctx, competing)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("competing create did not wait: %v", err)
	}
	content, err := os.ReadFile(file)
	if err != nil || string(content) != "initial" {
		t.Fatalf("files changed before pull completed: %q %v", content, err)
	}
	close(releasePull)
	released = true
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := second.Create(context.Background(), competing); err == nil {
		t.Fatal("accepted create over existing container")
	}
	content, err = os.ReadFile(file)
	if err != nil || string(content) != "first" || creates.Load() != 1 {
		t.Fatalf("duplicate create changed files: %q count=%d err=%v", content, creates.Load(), err)
	}
}

func TestInfoReportsDaemonArchitecture(t *testing.T) {
	adapter := testAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/info") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ServerVersion":"test-daemon","ContainersRunning":3,"Architecture":"aarch64"}`)
	})
	info, err := adapter.Info(context.Background())
	if err != nil || info.Architecture != "arm64" || info.Version != "test-daemon" || info.RunningContainers != 3 {
		t.Fatalf("daemon info=%+v err=%v", info, err)
	}
}
