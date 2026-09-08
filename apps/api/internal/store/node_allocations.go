package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/scheduling"
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
	if err := s.lockNodePorts(ctx, instance.NodeID); err != nil {
		return err
	}
	var assigned []domain.GameServer
	if err := s.db.WithContext(ctx).Where("node_id = ? AND id <> ?", instance.NodeID, instance.ID).Find(&assigned).Error; err != nil {
		return err
	}
	reserved := make([]scheduling.Resources, 0, len(assigned))
	for _, other := range assigned {
		reserved = append(reserved, scheduling.Resources{CPU: other.Spec.Resources.CPULimitCores, MemoryMB: int64(other.Spec.Resources.MemoryLimitMB)})
		if instance.Spec.Network.HostPort > 0 && other.Spec.Network.HostPort == instance.Spec.Network.HostPort {
			return fmt.Errorf("%w: host port %d is reserved", ErrNodeAllocationUnavailable, instance.Spec.Network.HostPort)
		}
	}
	if _, err := scheduling.CheckCapacity(
		scheduling.Resources{CPU: float64(node.CPUCores), MemoryMB: node.MemoryTotalMB},
		scheduling.Resources{CPU: instance.Spec.Resources.CPULimitCores, MemoryMB: int64(instance.Spec.Resources.MemoryLimitMB)}, reserved,
	); err != nil {
		return fmt.Errorf("%w: %w", ErrNodeAllocationUnavailable, err)
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
