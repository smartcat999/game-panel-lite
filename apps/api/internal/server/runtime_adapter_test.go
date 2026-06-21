package server

import (
	"context"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type captureWorkloadRuntime struct {
	spec      domain.WorkloadSpec
	startID   string
	stopID    string
	removeID  string
	inspectID string
	status    domain.WorkloadStatus
}

func (r *captureWorkloadRuntime) CreateWorkload(_ context.Context, spec domain.WorkloadSpec) (string, error) {
	r.spec = spec
	return "runtime-1", nil
}

func (r *captureWorkloadRuntime) StartWorkload(_ context.Context, runtimeID string) error {
	r.startID = runtimeID
	return nil
}

func (r *captureWorkloadRuntime) StopWorkload(_ context.Context, runtimeID string) error {
	r.stopID = runtimeID
	return nil
}

func (r *captureWorkloadRuntime) RemoveWorkload(_ context.Context, runtimeID string) error {
	r.removeID = runtimeID
	return nil
}

func (r *captureWorkloadRuntime) InspectWorkload(_ context.Context, runtimeID string) (domain.WorkloadStatus, error) {
	r.inspectID = runtimeID
	return r.status, nil
}

func TestRuntimeAdapterClientCreatesWorkload(t *testing.T) {
	adapter := &captureWorkloadRuntime{}
	client := NewRuntimeAdapterClient(adapter)

	spec := domain.WorkloadSpec{
		ServerID: "srv-1",
		Name:     "Friends",
		Image:    "game:latest",
		Network:  domain.WorkloadNetwork{Port: 7777, HostPort: 47777, Protocol: "tcp"},
	}
	runtimeID, err := client.Create(context.Background(), domain.WorkloadSpec{
		ServerID: spec.ServerID,
		Name:     spec.Name,
		Image:    spec.Image,
		Network:  spec.Network,
	})
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}
	if runtimeID != "runtime-1" {
		t.Fatalf("expected runtime ID, got %q", runtimeID)
	}
	if adapter.spec.ServerID != spec.ServerID || adapter.spec.Image != spec.Image {
		t.Fatalf("unexpected workload spec: %+v", adapter.spec)
	}
}

func TestRuntimeAdapterClientMapsLifecycleCalls(t *testing.T) {
	adapter := &captureWorkloadRuntime{status: domain.WorkloadStatus{RuntimeID: "runtime-4", State: domain.ActualRunning}}
	client := NewRuntimeAdapterClient(adapter)

	if err := client.Start(context.Background(), "runtime-1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := client.Stop(context.Background(), "runtime-2"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := client.Remove(context.Background(), "runtime-3"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	status, err := client.Inspect(context.Background(), "runtime-4")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if adapter.startID != "runtime-1" || adapter.stopID != "runtime-2" || adapter.removeID != "runtime-3" || adapter.inspectID != "runtime-4" {
		t.Fatalf("lifecycle IDs not forwarded: %+v", adapter)
	}
	if status.State != domain.ActualRunning {
		t.Fatalf("expected actual running, got %q", status.State)
	}
}
