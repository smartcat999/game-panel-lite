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
func (a *countingAdapter) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	return domain.RuntimeImageStatus{Image: image, Status: ImageStatusReady}
}
func (a *countingAdapter) PrepareImage(context.Context, string) error { return nil }
func (a *countingAdapter) CreateWorkload(context.Context, domain.WorkloadSpec) (string, error) {
	return "", nil
}
func (a *countingAdapter) StartWorkload(context.Context, string) error  { return nil }
func (a *countingAdapter) StopWorkload(context.Context, string) error   { return nil }
func (a *countingAdapter) RemoveWorkload(context.Context, string) error { return nil }
func (a *countingAdapter) InspectWorkload(context.Context, string) (domain.WorkloadStatus, error) {
	return domain.WorkloadStatus{}, nil
}
func (a *countingAdapter) StatsWorkload(context.Context, string) (WorkloadStats, error) {
	return WorkloadStats{}, nil
}
func (a *countingAdapter) HostStats(context.Context) (HostStats, error) {
	return HostStats{}, nil
}
func (a *countingAdapter) LogsWorkload(context.Context, string, bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (a *countingAdapter) LogSnapshotWorkload(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (a *countingAdapter) SendCommandWorkload(context.Context, string, string) error {
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
