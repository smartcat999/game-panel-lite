package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/scheduling"
	"gorm.io/gorm"
)

func regionalNodeReady(node regional.Node, observed regionalNodeSessionRow, now int64, maxAge time.Duration) bool {
	return node.Schedulable && observed.Epoch > 0 && observed.Sequence > 0 && observed.RuntimeReady && observed.Architecture == node.Architecture && observed.LastSeenMS > 0 && observed.LastSeenMS <= now && now-observed.LastSeenMS <= maxAge.Milliseconds()
}

// RegionalCapacityCandidates reads a bounded, consistent regional snapshot.
// It uses batched IDs and Go composition, never a JOIN or per-node query loop.
func (s *RegionalStore) RegionalCapacityCandidates(ctx context.Context, query regional.CandidateQuery, maxHeartbeatAge time.Duration) ([]regional.Candidate, error) {
	if query.RegionID != s.regionID {
		return nil, ErrRegionMismatch
	}
	if len(query.AllowedNodeIDs) < 1 || len(query.AllowedNodeIDs) > 200 || query.Architecture == "" || len(query.Architecture) > 128 || maxHeartbeatAge < time.Millisecond || maxHeartbeatAge > time.Hour {
		return nil, regional.ErrInvalidNode
	}
	needed := scheduling.Resources{CPU: query.Resources.CPU, MemoryMB: query.Resources.MemoryMB}
	if _, err := scheduling.CheckCapacity(needed, needed, nil); err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(query.AllowedNodeIDs))
	for _, id := range query.AllowedNodeIDs {
		if id == "" || len(id) > 128 || id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n") || allowed[id] {
			return nil, regional.ErrInvalidNode
		}
		allowed[id] = true
	}
	ids := append([]string(nil), query.AllowedNodeIDs...)
	if query.RequiredNodeID != "" {
		if !allowed[query.RequiredNodeID] {
			return nil, regional.ErrNodeUnavailable
		}
		ids = []string{query.RequiredNodeID}
	}
	candidates := make([]regional.Candidate, 0)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var nodes []regional.Node
		if err := tx.Table("regional_nodes").Where("id IN ? AND schedulable = ? AND architecture = ?", ids, true, query.Architecture).Order("id").Find(&nodes).Error; err != nil {
			return err
		}
		if len(nodes) == 0 {
			return nil
		}
		selected := make([]string, 0, len(nodes))
		for _, node := range nodes {
			selected = append(selected, node.ID)
		}
		var sessions []regionalNodeSessionRow
		if err := tx.Table("regional_node_sessions").Where("node_id IN ?", selected).Find(&sessions).Error; err != nil {
			return err
		}
		byNode := make(map[string]regionalNodeSessionRow, len(sessions))
		for _, session := range sessions {
			byNode[session.NodeID] = session
		}
		var usage []struct {
			NodeID   string
			CPU      float64
			MemoryMB int64
		}
		if err := tx.Table("regional_allocations").Select("node_id,SUM(cpu) AS cpu,SUM(memory_mb) AS memory_mb").Where("node_id IN ? AND status = ?", selected, "reserved").Group("node_id").Find(&usage).Error; err != nil {
			return err
		}
		reserved := make(map[string][]scheduling.Resources, len(usage))
		for _, used := range usage {
			reserved[used.NodeID] = []scheduling.Resources{{CPU: used.CPU, MemoryMB: used.MemoryMB}}
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			observed := byNode[node.ID]
			if !regionalNodeReady(node, observed, now, maxHeartbeatAge) {
				continue
			}
			remaining, err := scheduling.CheckCapacity(scheduling.Resources{CPU: node.CPU, MemoryMB: node.MemoryMB}, needed, reserved[node.ID])
			if err != nil {
				continue
			}
			candidates = append(candidates, regional.Candidate{NodeID: node.ID, NodeVersion: node.Version, SessionEpoch: observed.Epoch, RemainingCPU: remaining.CPU, RemainingMemoryMB: remaining.MemoryMB})
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return candidates, nil
}
