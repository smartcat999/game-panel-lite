package store

import (
	"context"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
)

// ListRegionalNodeOperations composes a bounded operational view from
// Region-owned tables. The component queries are batched and never use JOIN.
func (s *RegionalStore) ListRegionalNodeOperations(ctx context.Context, after string, limit int, heartbeatFreshness time.Duration) (regional.NodeOperationsPage, error) {
	if limit < 1 || limit > 200 || len(after) > 128 || heartbeatFreshness < time.Second || heartbeatFreshness > time.Hour {
		return regional.NodeOperationsPage{}, regional.ErrInvalidNodeOperations
	}
	page := regional.NodeOperationsPage{RegionID: s.regionID, Nodes: make([]regional.NodeOperations, 0)}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		page.ObservedAtMS = now
		var nodes []regional.Node
		if err := tx.Table("regional_nodes").Where("id > ?", after).Order("id").Limit(limit + 1).Find(&nodes).Error; err != nil {
			return err
		}
		if len(nodes) > limit {
			nodes = nodes[:limit]
			page.NextCursor = nodes[len(nodes)-1].ID
		}
		if len(nodes) == 0 {
			return nil
		}
		ids := make([]string, len(nodes))
		for i := range nodes {
			ids[i] = nodes[i].ID
		}
		var sessions []regionalNodeSessionRow
		if err := tx.Table("regional_node_sessions").Where("node_id IN ?", ids).Find(&sessions).Error; err != nil {
			return err
		}
		type aggregate struct {
			NodeID   string
			CPU      float64
			MemoryMB int64
			Count    int64
		}
		var allocations []aggregate
		if err := tx.Table("regional_allocations").Select("node_id, COALESCE(SUM(cpu),0) AS cpu, COALESCE(SUM(memory_mb),0) AS memory_mb, COUNT(*) AS count").Where("node_id IN ? AND status = ?", ids, "reserved").Group("node_id").Find(&allocations).Error; err != nil {
			return err
		}
		var tasks []aggregate
		if err := tx.Table("regional_node_tasks").Select("node_id, COUNT(*) AS count").Where("node_id IN ? AND status = ?", ids, "awaiting_authority").Group("node_id").Find(&tasks).Error; err != nil {
			return err
		}
		sessionByNode := make(map[string]regionalNodeSessionRow, len(sessions))
		for _, session := range sessions {
			sessionByNode[session.NodeID] = session
		}
		allocationByNode := make(map[string]aggregate, len(allocations))
		for _, allocation := range allocations {
			allocationByNode[allocation.NodeID] = allocation
		}
		tasksByNode := make(map[string]int64, len(tasks))
		for _, task := range tasks {
			tasksByNode[task.NodeID] = task.Count
		}
		cutoff := now - heartbeatFreshness.Milliseconds()
		for _, node := range nodes {
			session := sessionByNode[node.ID]
			reserved := allocationByNode[node.ID]
			page.Nodes = append(page.Nodes, regional.NodeOperations{Node: node, Online: session.RuntimeReady && session.LastSeenMS >= cutoff, RuntimeReady: session.RuntimeReady, LastSeenMS: session.LastSeenMS, SessionEpoch: session.Epoch, ReservedCPU: reserved.CPU, ReservedMemoryMB: reserved.MemoryMB, Allocations: reserved.Count, PendingTasks: tasksByNode[node.ID]})
		}
		return nil
	})
	if err != nil {
		return regional.NodeOperationsPage{}, err
	}
	if err := page.Validate(); err != nil {
		return regional.NodeOperationsPage{}, err
	}
	return page, nil
}
