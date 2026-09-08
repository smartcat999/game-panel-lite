package store

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

var ErrNodeAllocationUnavailable = errors.New("node allocation unavailable")

// lockNodeAllocation must run inside the transaction that writes placement.
// Workspace authorization, when needed, precedes this lock. Assigned instances
// retain capacity and ports until their database row is removed; desired deletion
// alone is not evidence that their containers have released resources.
func (s *Store) lockNodeAllocation(ctx context.Context, instance domain.GameServer) error {
	if instance.NodeID == "" {
		return nil
	}
	locked := s.db.WithContext(ctx).Model(&domain.ComputeNode{}).Where("id = ?", instance.NodeID).UpdateColumn("updated_at", gorm.Expr("updated_at"))
	if locked.Error != nil {
		return locked.Error
	}
	if locked.RowsAffected != 1 {
		return fmt.Errorf("%w: target node is missing", ErrNodeAllocationUnavailable)
	}
	node, err := s.GetComputeNode(ctx, instance.NodeID)
	if err != nil {
		return err
	}
	if node.Unschedulable {
		return fmt.Errorf("%w: target node is not accepting allocations", ErrNodeAllocationUnavailable)
	}
	if !finiteResources(instance.Spec.Resources) || node.CPUCores <= 0 || node.MemoryTotalMB <= 0 {
		return fmt.Errorf("%w: finite instance limits and known node capacity are required", ErrNodeAllocationUnavailable)
	}
	if err := s.lockNodePorts(ctx, instance.NodeID); err != nil {
		return err
	}
	var assigned []domain.GameServer
	if err := s.db.WithContext(ctx).Where("node_id = ? AND id <> ?", instance.NodeID, instance.ID).Find(&assigned).Error; err != nil {
		return err
	}
	remainingCPU, remainingMemory := float64(node.CPUCores), node.MemoryTotalMB
	for _, other := range assigned {
		if !finiteResources(other.Spec.Resources) {
			return fmt.Errorf("%w: existing instance has unbounded resource limits", ErrNodeAllocationUnavailable)
		}
		remainingCPU -= other.Spec.Resources.CPULimitCores
		// Subtract only after comparing to avoid overflowing aggregate memory sums.
		memory := int64(other.Spec.Resources.MemoryLimitMB)
		if memory > remainingMemory {
			return fmt.Errorf("%w: node memory is already fully allocated", ErrNodeAllocationUnavailable)
		}
		remainingMemory -= memory
		if instance.Spec.Network.HostPort > 0 && other.Spec.Network.HostPort == instance.Spec.Network.HostPort {
			return fmt.Errorf("%w: host port %d is reserved", ErrNodeAllocationUnavailable, instance.Spec.Network.HostPort)
		}
	}
	if math.IsNaN(remainingCPU) || instance.Spec.Resources.CPULimitCores > remainingCPU || int64(instance.Spec.Resources.MemoryLimitMB) > remainingMemory {
		return fmt.Errorf("%w: insufficient node capacity", ErrNodeAllocationUnavailable)
	}
	host := instance.Spec.Network.HostPort
	if host == 0 {
		host = instance.Spec.Network.Port
	}
	if host > 0 {
		return s.reserveNodePorts(ctx, instance.NodeID, instance.ID, []int{host})
	}
	return nil
}
