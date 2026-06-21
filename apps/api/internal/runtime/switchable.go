package runtime

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type SwitchableAdapter struct {
	mu      sync.RWMutex
	adapter Adapter
}

func NewSwitchableAdapter(adapter Adapter) *SwitchableAdapter {
	return &SwitchableAdapter{adapter: adapter}
}

func (s *SwitchableAdapter) Set(adapter Adapter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adapter = adapter
}

func (s *SwitchableAdapter) current() Adapter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.adapter
}

func (s *SwitchableAdapter) Check(ctx context.Context) DockerStatus {
	return s.current().Check(ctx)
}

func (s *SwitchableAdapter) ImageStatus(ctx context.Context, image string) domain.RuntimeImageStatus {
	return s.current().ImageStatus(ctx, image)
}

func (s *SwitchableAdapter) PrepareImage(ctx context.Context, image string) error {
	return s.current().PrepareImage(ctx, image)
}

func (s *SwitchableAdapter) PrepareImageWithProgress(ctx context.Context, image string, onProgress ImagePrepareProgressFunc) error {
	current := s.current()
	if preparer, ok := current.(ImageProgressPreparer); ok {
		return preparer.PrepareImageWithProgress(ctx, image, onProgress)
	}
	return current.PrepareImage(ctx, image)
}

func (s *SwitchableAdapter) SaveImageArchive(ctx context.Context, image string, path string) error {
	if manager, ok := s.current().(ImageArchiveManager); ok {
		return manager.SaveImageArchive(ctx, image, path)
	}
	return fmt.Errorf("runtime adapter does not support local image archives")
}

func (s *SwitchableAdapter) LoadImageArchive(ctx context.Context, path string) error {
	if manager, ok := s.current().(ImageArchiveManager); ok {
		return manager.LoadImageArchive(ctx, path)
	}
	return fmt.Errorf("runtime adapter does not support local image archives")
}

func (s *SwitchableAdapter) CreateWorkload(ctx context.Context, spec domain.WorkloadSpec) (string, error) {
	return s.current().CreateWorkload(ctx, spec)
}

func (s *SwitchableAdapter) StartWorkload(ctx context.Context, runtimeID string) error {
	return s.current().StartWorkload(ctx, runtimeID)
}

func (s *SwitchableAdapter) StopWorkload(ctx context.Context, runtimeID string) error {
	return s.current().StopWorkload(ctx, runtimeID)
}

func (s *SwitchableAdapter) RemoveWorkload(ctx context.Context, runtimeID string) error {
	return s.current().RemoveWorkload(ctx, runtimeID)
}

func (s *SwitchableAdapter) InspectWorkload(ctx context.Context, runtimeID string) (domain.WorkloadStatus, error) {
	return s.current().InspectWorkload(ctx, runtimeID)
}

func (s *SwitchableAdapter) StatsWorkload(ctx context.Context, runtimeID string) (WorkloadStats, error) {
	return s.current().StatsWorkload(ctx, runtimeID)
}

func (s *SwitchableAdapter) HostStats(ctx context.Context) (HostStats, error) {
	return s.current().HostStats(ctx)
}

func (s *SwitchableAdapter) LogsWorkload(ctx context.Context, runtimeID string, follow bool) (io.ReadCloser, error) {
	return s.current().LogsWorkload(ctx, runtimeID, follow)
}

func (s *SwitchableAdapter) LogSnapshotWorkload(ctx context.Context, runtimeID string) (io.ReadCloser, error) {
	return s.current().LogSnapshotWorkload(ctx, runtimeID)
}

func (s *SwitchableAdapter) SendCommandWorkload(ctx context.Context, runtimeID string, command string) error {
	return s.current().SendCommandWorkload(ctx, runtimeID, command)
}
