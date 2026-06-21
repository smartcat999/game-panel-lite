package runtime

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type testAdapter struct {
	status DockerStatus
	spec   ContainerSpec
	ids    []string
}

func (a testAdapter) Check(context.Context) DockerStatus { return a.status }
func (a testAdapter) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	return domain.RuntimeImageStatus{Image: image, Status: ImageStatusReady}
}
func (a testAdapter) PrepareImage(context.Context, string) error { return nil }
func (a *testAdapter) CreateWorkload(_ context.Context, spec domain.WorkloadSpec) (string, error) {
	a.spec = ContainerSpecFromWorkload(spec)
	return "runtime-1", nil
}
func (a *testAdapter) StartWorkload(_ context.Context, runtimeID string) error {
	a.ids = append(a.ids, "start:"+runtimeID)
	return nil
}
func (a *testAdapter) StopWorkload(_ context.Context, runtimeID string) error {
	a.ids = append(a.ids, "stop:"+runtimeID)
	return nil
}
func (a *testAdapter) RemoveWorkload(_ context.Context, runtimeID string) error {
	a.ids = append(a.ids, "remove:"+runtimeID)
	return nil
}
func (a *testAdapter) InspectWorkload(_ context.Context, runtimeID string) (domain.WorkloadStatus, error) {
	a.ids = append(a.ids, "inspect:"+runtimeID)
	return domain.WorkloadStatus{RuntimeID: runtimeID, State: domain.ActualStopped}, nil
}
func (a testAdapter) StatsWorkload(context.Context, string) (WorkloadStats, error) {
	return WorkloadStats{}, nil
}
func (a testAdapter) HostStats(context.Context) (HostStats, error) {
	return HostStats{}, nil
}
func (a testAdapter) LogsWorkload(context.Context, string, bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (a testAdapter) LogSnapshotWorkload(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (a testAdapter) SendCommandWorkload(context.Context, string, string) error {
	return nil
}

func TestSwitchableAdapterSetChangesDelegatedRuntime(t *testing.T) {
	switchable := NewSwitchableAdapter(&testAdapter{
		status: DockerStatus{Available: false, Message: "first", Host: "unix:///first.sock"},
	})

	if got := switchable.Check(context.Background()); got.Host != "unix:///first.sock" {
		t.Fatalf("expected first host, got %q", got.Host)
	}

	switchable.Set(&testAdapter{
		status: DockerStatus{Available: true, Message: "second", Host: "unix:///second.sock"},
	})

	got := switchable.Check(context.Background())
	if !got.Available || got.Host != "unix:///second.sock" {
		t.Fatalf("expected switched available host, got %+v", got)
	}
}

func TestContainerSpecFromWorkloadSplitsLegacyConfigFile(t *testing.T) {
	spec := ContainerSpecFromWorkload(domain.WorkloadSpec{
		ServerID: "srv-1",
		Name:     "Friends",
		Image:    "game:latest",
		Network:  domain.WorkloadNetwork{Port: 7777, HostPort: 47777, Protocol: "tcp"},
		Resources: domain.WorkloadResources{
			CPULimitCores: 1.5,
			MemoryLimitMB: 2048,
		},
		DataDir: "/tmp/gamepanel",
		Options: domain.WorkloadOptions{
			Env:        []string{"A=B"},
			Cmd:        []string{"run"},
			DataMounts: []string{"/data"},
			Files: map[string]string{
				"serverconfig.txt": "config",
				"extra.txt":        "extra",
			},
		},
	})
	if spec.InstanceID != "srv-1" || spec.Image != "game:latest" {
		t.Fatalf("unexpected container spec: %+v", spec)
	}
	if spec.ConfigText != "config" {
		t.Fatalf("expected config text to be split out, got %q", spec.ConfigText)
	}
	if spec.Options.Files["extra.txt"] != "extra" {
		t.Fatalf("expected extra file to remain, got %+v", spec.Options.Files)
	}
	if _, ok := spec.Options.Files["serverconfig.txt"]; ok {
		t.Fatal("serverconfig.txt should not be duplicated in options files")
	}
}

func TestSwitchableAdapterBridgesWorkloadLifecycle(t *testing.T) {
	adapter := &testAdapter{}
	switchable := NewSwitchableAdapter(adapter)

	runtimeID, err := switchable.CreateWorkload(context.Background(), domain.WorkloadSpec{ServerID: "srv-1", Image: "game:latest"})
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}
	if runtimeID != "runtime-1" || adapter.spec.InstanceID != "srv-1" {
		t.Fatalf("unexpected create result id=%q spec=%+v", runtimeID, adapter.spec)
	}
	if err := switchable.StartWorkload(context.Background(), "runtime-1"); err != nil {
		t.Fatalf("start workload: %v", err)
	}
	if err := switchable.StopWorkload(context.Background(), "runtime-2"); err != nil {
		t.Fatalf("stop workload: %v", err)
	}
	if err := switchable.RemoveWorkload(context.Background(), "runtime-3"); err != nil {
		t.Fatalf("remove workload: %v", err)
	}
	status, err := switchable.InspectWorkload(context.Background(), "runtime-4")
	if err != nil {
		t.Fatalf("inspect workload: %v", err)
	}
	if status.State != domain.ActualStopped {
		t.Fatalf("expected actual stopped, got %q", status.State)
	}
	expected := []string{"start:runtime-1", "stop:runtime-2", "remove:runtime-3", "inspect:runtime-4"}
	if strings.Join(adapter.ids, ",") != strings.Join(expected, ",") {
		t.Fatalf("expected ids %v, got %v", expected, adapter.ids)
	}
}
