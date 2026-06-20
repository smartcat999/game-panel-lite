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

func (m *MockAdapter) Create(_ context.Context, spec ContainerSpec) (string, error) {
	return "mock-" + spec.InstanceID, nil
}

func (m *MockAdapter) Start(context.Context, domain.GameServerInstance) error   { return nil }
func (m *MockAdapter) Stop(context.Context, domain.GameServerInstance) error    { return nil }
func (m *MockAdapter) Restart(context.Context, domain.GameServerInstance) error { return nil }
func (m *MockAdapter) Remove(context.Context, domain.GameServerInstance) error  { return nil }

func (m *MockAdapter) Inspect(context.Context, domain.GameServerInstance) (domain.ServerStatus, error) {
	return domain.StatusStopped, nil
}

func (m *MockAdapter) Stats(context.Context, domain.GameServerInstance) (ContainerStats, error) {
	return ContainerStats{}, nil
}
func (m *MockAdapter) HostStats(context.Context) (HostStats, error) {
	return HostStats{}, nil
}

func (m *MockAdapter) Logs(_ context.Context, instance domain.GameServerInstance) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("[Info] Mock Terraria log stream for " + instance.Name + "\n")), nil
}

func (m *MockAdapter) SendCommand(context.Context, domain.GameServerInstance, string) error {
	return nil
}
