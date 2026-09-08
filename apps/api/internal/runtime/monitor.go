package runtime

import (
	"context"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"sync"
	"time"
)

type DockerMonitor struct {
	runtime *SwitchableAdapter
	mu      sync.RWMutex
	status  DockerStatus
}

func NewDockerMonitor(adapter *SwitchableAdapter) *DockerMonitor {
	return &DockerMonitor{
		runtime: adapter,
		status: DockerStatus{
			Available: false,
			Message:   "Docker status has not been checked yet",
		},
	}
}

func (m *DockerMonitor) Status() DockerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *DockerMonitor) Refresh(ctx context.Context) DockerStatus {
	status := m.runtime.Check(ctx)
	status.LastCheckedAt = time.Now().UTC()

	m.mu.Lock()
	m.status = status
	m.mu.Unlock()

	return status
}

func (m *DockerMonitor) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Refresh(ctx)
		}
	}
}

// Architecture returns recent daemon evidence, never the control-plane host's
// architecture. Unavailable or stale probes cannot authorize placement.
func (m *DockerMonitor) Architecture() string {
	status := m.Status()
	if !status.Available || status.LastCheckedAt.IsZero() || time.Since(status.LastCheckedAt) > 30*time.Second {
		return ""
	}
	return workload.NormalizeArchitecture(status.Architecture)
}
