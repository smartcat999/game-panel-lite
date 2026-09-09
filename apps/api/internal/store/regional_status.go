package store

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionstatus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *RegionalStore) RegionStatusOutbox() delivery.Outbox {
	return &sqlOutbox{db: s.db, table: "regional_status_outbox", region: s.regionID}
}

// CaptureRegionStatus reads Region-owned tables in one short transaction and
// persists the resulting observation before any broker I/O. Each query is a
// bounded aggregate and no query joins tables.
func (s *RegionalStore) CaptureRegionStatus(ctx context.Context, heartbeatFreshness time.Duration) (regionstatus.Snapshot, error) {
	if heartbeatFreshness < time.Second || heartbeatFreshness > time.Hour {
		return regionstatus.Snapshot{}, regionstatus.ErrInvalidSnapshot
	}
	var snapshot regionstatus.Snapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		snapshot = regionstatus.Snapshot{SchemaVersion: 1, EventID: uuid.NewString(), RegionID: s.regionID, ObservedAtMS: now}
		var nodeTotals struct {
			Total, Schedulable, MemoryTotalMB int64
			CPUTotal                          float64
		}
		if err := tx.Table("regional_nodes").Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN schedulable THEN 1 ELSE 0 END),0) AS schedulable, COALESCE(SUM(cpu),0) AS cpu_total, COALESCE(SUM(memory_mb),0) AS memory_total_mb").Take(&nodeTotals).Error; err != nil {
			return err
		}
		snapshot.Nodes.Total, snapshot.Nodes.Schedulable = nodeTotals.Total, nodeTotals.Schedulable
		snapshot.Capacity.CPUTotal, snapshot.Capacity.MemoryTotalMB = nodeTotals.CPUTotal, nodeTotals.MemoryTotalMB
		if err := tx.Table("regional_node_sessions").Where("runtime_ready = ? AND last_seen_ms >= ?", true, now-heartbeatFreshness.Milliseconds()).Count(&snapshot.Nodes.Online).Error; err != nil {
			return err
		}
		var reserved struct {
			CPUReserved      float64
			MemoryReservedMB int64
		}
		if err := tx.Table("regional_allocations").Select("COALESCE(SUM(cpu),0) AS cpu_reserved, COALESCE(SUM(memory_mb),0) AS memory_reserved_mb").Where("status = ?", "reserved").Take(&reserved).Error; err != nil {
			return err
		}
		snapshot.Capacity.CPUReserved, snapshot.Capacity.MemoryReservedMB = reserved.CPUReserved, reserved.MemoryReservedMB
		var deployments []struct {
			SchedulingStatus string
			Count            int64
		}
		if err := tx.Table("regional_deployments").Select("scheduling_status, COUNT(*) AS count").Group("scheduling_status").Find(&deployments).Error; err != nil {
			return err
		}
		for _, item := range deployments {
			snapshot.Deployments.Total += item.Count
			switch item.SchedulingStatus {
			case "pending":
				snapshot.Deployments.Pending += item.Count
			case "reserved":
				snapshot.Deployments.Reserved += item.Count
			case "rejected":
				snapshot.Deployments.Rejected += item.Count
			default:
				return regionstatus.ErrInvalidSnapshot
			}
		}
		if err := tx.Table("regional_node_tasks").Where("status = ?", "awaiting_authority").Count(&snapshot.Tasks.AwaitingAuthority).Error; err != nil {
			return err
		}
		var state struct{ Sequence int64 }
		if err := tx.Table("regional_status_sequence").Where("id = 1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&state).Error; err != nil {
			return err
		}
		if state.Sequence == math.MaxInt64 {
			return regionstatus.ErrInvalidSnapshot
		}
		snapshot.Sequence = state.Sequence + 1
		if err := snapshot.Validate(); err != nil {
			return err
		}
		payload, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		if err := tx.Table("regional_status_sequence").Where("id = 1 AND sequence = ?", state.Sequence).Update("sequence", snapshot.Sequence).Error; err != nil {
			return err
		}
		row := map[string]any{"id": snapshot.EventID, "sequence": snapshot.Sequence, "event_type": "region.status.observed", "payload": string(payload)}
		return tx.Table("regional_status_outbox").Create(row).Error
	})
	if err != nil {
		return regionstatus.Snapshot{}, err
	}
	return snapshot, nil
}
