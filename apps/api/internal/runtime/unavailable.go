package runtime

import (
	"context"
	"fmt"
	"io"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type UnavailableAdapter struct {
	err error
}

func NewUnavailableAdapter(err error) *UnavailableAdapter {
	if err == nil {
		err = fmt.Errorf("runtime unavailable")
	}
	return &UnavailableAdapter{err: err}
}

func (a *UnavailableAdapter) Check(context.Context) DockerStatus {
	return DockerStatus{Available: false, Message: a.err.Error()}
}

func (a *UnavailableAdapter) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	return domain.RuntimeImageStatus{Image: image, Status: ImageStatusFailed, Message: a.err.Error()}
}

func (a *UnavailableAdapter) PrepareImage(context.Context, string) error {
	return a.err
}

func (a *UnavailableAdapter) CreateWorkload(context.Context, domain.WorkloadSpec) (string, error) {
	return "", a.err
}

func (a *UnavailableAdapter) StartWorkload(context.Context, string) error {
	return a.err
}

func (a *UnavailableAdapter) StopWorkload(context.Context, string) error {
	return a.err
}

func (a *UnavailableAdapter) RemoveWorkload(context.Context, string) error {
	return a.err
}

func (a *UnavailableAdapter) InspectWorkload(context.Context, string) (domain.WorkloadStatus, error) {
	return domain.WorkloadStatus{}, a.err
}

func (a *UnavailableAdapter) StatsWorkload(context.Context, string) (WorkloadStats, error) {
	return WorkloadStats{}, a.err
}

func (a *UnavailableAdapter) HostStats(context.Context) (HostStats, error) {
	return HostStats{}, a.err
}

func (a *UnavailableAdapter) LogsWorkload(context.Context, string, bool) (io.ReadCloser, error) {
	return nil, a.err
}

func (a *UnavailableAdapter) LogSnapshotWorkload(context.Context, string) (io.ReadCloser, error) {
	return nil, a.err
}

func (a *UnavailableAdapter) SendCommandWorkload(context.Context, string, string) error {
	return a.err
}
