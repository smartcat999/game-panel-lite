package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionstatus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type globalRegionStatusRow struct {
	RegionID, EventID, PayloadHash         string
	Sequence, ObservedAtMS                 int64
	NodeTotal, NodeOnline, NodeSchedulable int64
	CPUTotal, CPUReserved                  float64
	MemoryTotalMB, MemoryReservedMB        int64
	DeploymentTotal, DeploymentPending     int64
	DeploymentReserved, DeploymentRejected int64
	TaskAwaitingAuthority                  int64
}

func (r globalRegionStatusRow) snapshot() regionstatus.Snapshot {
	return regionstatus.Snapshot{SchemaVersion: 1, EventID: r.EventID, RegionID: r.RegionID, Sequence: r.Sequence, ObservedAtMS: r.ObservedAtMS,
		Nodes:       regionstatus.NodeSummary{Total: r.NodeTotal, Online: r.NodeOnline, Schedulable: r.NodeSchedulable},
		Capacity:    regionstatus.CapacitySummary{CPUTotal: r.CPUTotal, CPUReserved: r.CPUReserved, MemoryTotalMB: r.MemoryTotalMB, MemoryReservedMB: r.MemoryReservedMB},
		Deployments: regionstatus.DeploymentSummary{Total: r.DeploymentTotal, Pending: r.DeploymentPending, Reserved: r.DeploymentReserved, Rejected: r.DeploymentRejected},
		Tasks:       regionstatus.TaskSummary{AwaitingAuthority: r.TaskAwaitingAuthority}}
}

func regionStatusRow(snapshot regionstatus.Snapshot, hash string) globalRegionStatusRow {
	return globalRegionStatusRow{RegionID: snapshot.RegionID, EventID: snapshot.EventID, PayloadHash: hash, Sequence: snapshot.Sequence, ObservedAtMS: snapshot.ObservedAtMS,
		NodeTotal: snapshot.Nodes.Total, NodeOnline: snapshot.Nodes.Online, NodeSchedulable: snapshot.Nodes.Schedulable,
		CPUTotal: snapshot.Capacity.CPUTotal, CPUReserved: snapshot.Capacity.CPUReserved, MemoryTotalMB: snapshot.Capacity.MemoryTotalMB, MemoryReservedMB: snapshot.Capacity.MemoryReservedMB,
		DeploymentTotal: snapshot.Deployments.Total, DeploymentPending: snapshot.Deployments.Pending, DeploymentReserved: snapshot.Deployments.Reserved, DeploymentRejected: snapshot.Deployments.Rejected,
		TaskAwaitingAuthority: snapshot.Tasks.AwaitingAuthority}
}

// RecordRegionStatus accepts observations only from the authenticated source
// Region. Older messages are safe to acknowledge after a newer projection.
func (s *Store) RecordRegionStatus(ctx context.Context, source string, snapshot regionstatus.Snapshot) error {
	if snapshot.Validate() != nil || source == "" || source != snapshot.RegionID {
		return regionstatus.ErrInvalidSnapshot
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(payload))
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var directory regions.Entry
		if err := tx.Table("global_regions").Where("id = ?", source).Clauses(clause.Locking{Strength: "SHARE"}).Take(&directory).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return regionstatus.ErrSnapshotConflict
			}
			return err
		}
		var current globalRegionStatusRow
		err := tx.Table("global_region_statuses").Where("region_id = ?", source).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error
		if err == nil {
			if snapshot.Sequence < current.Sequence {
				return nil
			}
			if snapshot.Sequence == current.Sequence {
				if snapshot.EventID == current.EventID && hash == current.PayloadHash {
					return nil
				}
				return regionstatus.ErrSnapshotConflict
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row := regionStatusRow(snapshot, hash)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Table("global_region_statuses").Create(&row).Error
		}
		updated := tx.Table("global_region_statuses").Where("region_id = ? AND sequence = ?", source, current.Sequence).Updates(map[string]any{
			"sequence": snapshot.Sequence, "event_id": snapshot.EventID, "payload_hash": hash, "observed_at_ms": snapshot.ObservedAtMS,
			"node_total": snapshot.Nodes.Total, "node_online": snapshot.Nodes.Online, "node_schedulable": snapshot.Nodes.Schedulable,
			"cpu_total": snapshot.Capacity.CPUTotal, "cpu_reserved": snapshot.Capacity.CPUReserved,
			"memory_total_mb": snapshot.Capacity.MemoryTotalMB, "memory_reserved_mb": snapshot.Capacity.MemoryReservedMB,
			"deployment_total": snapshot.Deployments.Total, "deployment_pending": snapshot.Deployments.Pending,
			"deployment_reserved": snapshot.Deployments.Reserved, "deployment_rejected": snapshot.Deployments.Rejected,
			"task_awaiting_authority": snapshot.Tasks.AwaitingAuthority, "received_at": gorm.Expr("CURRENT_TIMESTAMP"),
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return regionstatus.ErrSnapshotConflict
		}
		return nil
	})
}

func (s *Store) GetRegionStatus(ctx context.Context, regionID string) (regionstatus.Snapshot, error) {
	var row globalRegionStatusRow
	err := s.db.WithContext(ctx).Table("global_region_statuses").Where("region_id = ?", regionID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return regionstatus.Snapshot{}, ErrNotFound
	}
	if err != nil {
		return regionstatus.Snapshot{}, err
	}
	return row.snapshot(), nil
}

func migrateSQLiteRegionStatus(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 18").Count(&count).Error; err != nil || count > 0 {
			return err
		}
		if err := tx.Exec(`CREATE TABLE global_region_statuses (
region_id text PRIMARY KEY REFERENCES global_regions(id), sequence integer NOT NULL CHECK(sequence > 0), event_id text NOT NULL UNIQUE,
payload_hash text NOT NULL, observed_at_ms integer NOT NULL CHECK(observed_at_ms > 0), node_total integer NOT NULL, node_online integer NOT NULL,
node_schedulable integer NOT NULL, cpu_total real NOT NULL, cpu_reserved real NOT NULL, memory_total_mb integer NOT NULL, memory_reserved_mb integer NOT NULL,
deployment_total integer NOT NULL, deployment_pending integer NOT NULL, deployment_reserved integer NOT NULL, deployment_rejected integer NOT NULL,
task_awaiting_authority integer NOT NULL, received_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP)`).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(18)").Error
	})
}
