package runtime

import (
	"context"
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

func (s *SwitchableAdapter) Create(ctx context.Context, spec ContainerSpec) (string, error) {
	return s.current().Create(ctx, spec)
}

func (s *SwitchableAdapter) Start(ctx context.Context, instance domain.GameServerInstance) error {
	return s.current().Start(ctx, instance)
}

func (s *SwitchableAdapter) Stop(ctx context.Context, instance domain.GameServerInstance) error {
	return s.current().Stop(ctx, instance)
}

func (s *SwitchableAdapter) Restart(ctx context.Context, instance domain.GameServerInstance) error {
	return s.current().Restart(ctx, instance)
}

func (s *SwitchableAdapter) Remove(ctx context.Context, instance domain.GameServerInstance) error {
	return s.current().Remove(ctx, instance)
}

func (s *SwitchableAdapter) Inspect(ctx context.Context, instance domain.GameServerInstance) (domain.ServerStatus, error) {
	return s.current().Inspect(ctx, instance)
}

func (s *SwitchableAdapter) Stats(ctx context.Context, instance domain.GameServerInstance) (ContainerStats, error) {
	return s.current().Stats(ctx, instance)
}

func (s *SwitchableAdapter) HostStats(ctx context.Context) (HostStats, error) {
	return s.current().HostStats(ctx)
}

func (s *SwitchableAdapter) Logs(ctx context.Context, instance domain.GameServerInstance) (io.ReadCloser, error) {
	return s.current().Logs(ctx, instance)
}

func (s *SwitchableAdapter) LogSnapshot(ctx context.Context, instance domain.GameServerInstance) (io.ReadCloser, error) {
	current := s.current()
	if snapshotter, ok := current.(LogSnapshotter); ok {
		return snapshotter.LogSnapshot(ctx, instance)
	}
	return current.Logs(ctx, instance)
}

func (s *SwitchableAdapter) SendCommand(ctx context.Context, instance domain.GameServerInstance, command string) error {
	return s.current().SendCommand(ctx, instance, command)
}
