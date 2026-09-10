package instancecontrol

import (
	"context"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

type DeploymentSummary struct {
	LogicalInstanceID    contract.LogicalInstanceID    `json:"logicalInstanceId"`
	RegionalDeploymentID contract.RegionalDeploymentID `json:"regionalDeploymentId"`
	RegionID             contract.RegionID             `json:"regionId"`
	Sequence             int64                         `json:"sequence"`
	ObservedState        string                        `json:"observedState"`
	ObservedAt           time.Time                     `json:"observedAt"`
}

func (m *Module) ApplyDeploymentSummary(_ context.Context, candidate DeploymentSummary) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.summaries[candidate.LogicalInstanceID]
	if candidate.Sequence <= current.Sequence {
		return false
	}
	m.summaries[candidate.LogicalInstanceID] = candidate
	if instance, ok := m.instances[candidate.LogicalInstanceID]; ok {
		instance = ApplySummary(instance, candidate)
		m.instances[candidate.LogicalInstanceID] = instance
	}
	return true
}

func ApplySummary(instance LogicalInstance, summary DeploymentSummary) LogicalInstance {
	switch summary.ObservedState {
	case "pending":
		instance.DeploymentState, instance.Stale = DeploymentWaitingRegion, false
	case "running":
		instance.DeploymentState, instance.Stale = DeploymentRunning, false
	case "stopped":
		instance.DeploymentState, instance.Stale = DeploymentStopped, false
	case "failed":
		instance.DeploymentState, instance.Stale = DeploymentFailed, false
	default:
		instance.Stale = true
	}
	return instance
}

func (m *Module) DeploymentSummary(_ context.Context, instanceID contract.LogicalInstanceID) (DeploymentSummary, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	summary, ok := m.summaries[instanceID]
	return summary, ok
}
