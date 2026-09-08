package docker

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// This opt-in test creates and removes only a uniquely named disposable workload.
func TestDockerIntegration(t *testing.T) {
	host := os.Getenv("GAMEPANEL_TEST_DOCKER_HOST")
	if host == "" {
		t.Skip("set GAMEPANEL_TEST_DOCKER_HOST to run against a disposable Docker workload")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	adapter, err := NewAdapter(host, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	id := "runtime-test-" + uuid.NewString()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		observed, err := adapter.Inspect(cleanup, id)
		if err != nil {
			t.Errorf("inspect disposable workload %s: %v", id, err)
			return
		}
		if observed.Exists {
			if observed.ServerID != id || observed.NodeID != "integration" {
				t.Errorf("unexpected cleanup ownership: %+v", observed)
				return
			}
			if err := adapter.Remove(cleanup, observed); err != nil {
				t.Errorf("remove disposable workload %s: %v", id, err)
			}
		}
	}()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	assignment := workload.Assignment{UID: uuid.NewString(), ServerID: id, NodeID: "integration", Generation: 1, DesiredState: "running", Spec: workload.Spec{
		ServerID: id, Image: "alpine:3.21", Resources: workload.Resources{CPULimitCores: 0.25, MemoryLimitMB: 64},
		Network: workload.Network{Port: 7777, HostPort: port, Protocol: "tcp"},
		Options: workload.Options{Cmd: []string{"sh", "-c", `echo ready; while read line; do echo "$line"; done`}, Files: map[string]string{"settings/test.ini": "isolated"}},
	}}
	for i := 0; i < 2; i++ {
		if got := worker.Reconcile(ctx, assignment, adapter); got.LastError != "" || got.ActualState != "running" {
			t.Fatalf("reconcile: %+v", got)
		}
	}
	inspect, err := adapter.client.ContainerInspect(ctx, "gamepanel-"+id)
	if err != nil {
		t.Fatal(err)
	}
	if inspect.HostConfig.NanoCPUs != 250000000 || inspect.HostConfig.Memory != 64*1024*1024 || inspect.HostConfig.NetworkMode == "host" {
		t.Fatalf("limits/isolation not applied: %+v", inspect.HostConfig)
	}
	if inspect.HostConfig.PortBindings["7777/tcp"][0].HostPort != strconv.Itoa(port) {
		t.Fatal("host port not applied")
	}
	if err := adapter.Console(ctx, id, "literal; echo not-a-shell-command"); err != nil {
		t.Fatal(err)
	}
	found := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		lines, err := adapter.Logs(ctx, inspect.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(lines, "\n"), "literal; echo not-a-shell-command") {
			found = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		t.Fatal("console input was not delivered literally to stdin")
	}
	observed, err := adapter.Inspect(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.ReadStoppedData(ctx, observed, func(context.Context, fs.FS) error { t.Error("running data exposed"); return nil }); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatal("running snapshot accepted", err)
	}
	assignment.DesiredState = "stopped"
	if got := worker.Reconcile(ctx, assignment, adapter); got.LastError != "" || got.ActualState != "stopped" {
		t.Fatalf("stop: %+v", got)
	}
	observed, err = adapter.Inspect(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.ReadStoppedData(ctx, observed, func(_ context.Context, files fs.FS) error {
		data, err := fs.ReadFile(files, "settings/test.ini")
		if err != nil {
			return err
		}
		if string(data) != "isolated" {
			t.Error("stopped data changed")
		}
		return nil
	}); err != nil {
		t.Fatal("stopped Docker data unavailable", err)
	}
	assignment.DesiredState = "deleted"
	if got := worker.Reconcile(ctx, assignment, adapter); got.LastError != "" || got.ActualState != "missing" {
		t.Fatalf("delete: %+v", got)
	}
}
