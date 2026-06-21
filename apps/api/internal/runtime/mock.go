package runtime

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type MockAdapter struct{}

func NewMockAdapter() *MockAdapter { return &MockAdapter{} }

func (m *MockAdapter) Check(context.Context) DockerStatus {
	return DockerStatus{Available: false, Message: "Docker runtime not connected in mock adapter", Host: "mock"}
}

func (m *MockAdapter) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	return domain.RuntimeImageStatus{Image: image, Status: ImageStatusReady}
}

func (m *MockAdapter) PrepareImage(context.Context, string) error {
	return nil
}

func (m *MockAdapter) SaveImageArchive(_ context.Context, image string, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("mock image archive: "+image+"\n"), 0o644)
}

func (m *MockAdapter) LoadImageArchive(context.Context, string) error {
	return nil
}

func (m *MockAdapter) CreateWorkload(_ context.Context, spec domain.WorkloadSpec) (string, error) {
	return "mock-" + spec.ServerID, nil
}

func (m *MockAdapter) StartWorkload(context.Context, string) error  { return nil }
func (m *MockAdapter) StopWorkload(context.Context, string) error   { return nil }
func (m *MockAdapter) RemoveWorkload(context.Context, string) error { return nil }

func (m *MockAdapter) InspectWorkload(_ context.Context, runtimeID string) (domain.WorkloadStatus, error) {
	return domain.WorkloadStatus{RuntimeID: runtimeID, State: domain.ActualStopped}, nil
}

func (m *MockAdapter) StatsWorkload(context.Context, string) (WorkloadStats, error) {
	return WorkloadStats{}, nil
}
func (m *MockAdapter) HostStats(context.Context) (HostStats, error) {
	return HostStats{}, nil
}

func (m *MockAdapter) LogsWorkload(_ context.Context, runtimeID string, _ bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("[Info] Mock log stream for " + runtimeID + "\n")), nil
}

func (m *MockAdapter) LogSnapshotWorkload(_ context.Context, runtimeID string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("[Info] Mock log stream for " + runtimeID + "\n")), nil
}

func (m *MockAdapter) SendCommandWorkload(context.Context, string, string) error {
	return nil
}
