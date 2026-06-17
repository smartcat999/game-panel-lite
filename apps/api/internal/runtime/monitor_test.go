package runtime

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type countingAdapter struct {
	checks int32
	status DockerStatus
}

func (a *countingAdapter) Check(context.Context) DockerStatus {
	atomic.AddInt32(&a.checks, 1)
	return a.status
}
func (a *countingAdapter) Create(context.Context, ContainerSpec) (string, error) {
	return "", nil
}
func (a *countingAdapter) Start(context.Context, domain.GameServerInstance) error   { return nil }
func (a *countingAdapter) Stop(context.Context, domain.GameServerInstance) error    { return nil }
func (a *countingAdapter) Restart(context.Context, domain.GameServerInstance) error { return nil }
func (a *countingAdapter) Remove(context.Context, domain.GameServerInstance) error  { return nil }
func (a *countingAdapter) Inspect(context.Context, domain.GameServerInstance) (domain.ServerStatus, error) {
	return domain.StatusStopped, nil
}
func (a *countingAdapter) Stats(context.Context, domain.GameServerInstance) (ContainerStats, error) {
	return ContainerStats{}, nil
}
func (a *countingAdapter) HostStats(context.Context) (HostStats, error) {
	return HostStats{}, nil
}
func (a *countingAdapter) Logs(context.Context, domain.GameServerInstance) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (a *countingAdapter) SendCommand(context.Context, domain.GameServerInstance, string) error {
	return nil
}

func TestDockerMonitorStatusReturnsCachedStatus(t *testing.T) {
	adapter := &countingAdapter{
		status: DockerStatus{Available: true, Message: "ok", Host: "unix:///docker.sock"},
	}
	monitor := NewDockerMonitor(NewSwitchableAdapter(adapter))

	if checks := atomic.LoadInt32(&adapter.checks); checks != 0 {
		t.Fatalf("expected no checks before refresh, got %d", checks)
	}

	got := monitor.Refresh(context.Background())
	if !got.Available || got.Host != "unix:///docker.sock" || got.LastCheckedAt.IsZero() {
		t.Fatalf("expected refreshed Docker status, got %+v", got)
	}

	_ = monitor.Status()
	_ = monitor.Status()
	if checks := atomic.LoadInt32(&adapter.checks); checks != 1 {
		t.Fatalf("expected cached status reads not to check Docker, got %d checks", checks)
	}
}
