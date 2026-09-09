package store

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionstatus"
)

func testRegionStatusProjection(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.RegisterRegion(ctx, "status-east", "Status East"); err != nil {
		t.Fatal(err)
	}
	status := regionstatus.Snapshot{SchemaVersion: 1, EventID: "status-1", RegionID: "status-east", Sequence: 1, ObservedAtMS: 100,
		Nodes: regionstatus.NodeSummary{Total: 2, Online: 1, Schedulable: 2}, Capacity: regionstatus.CapacitySummary{CPUTotal: 8, CPUReserved: 2, MemoryTotalMB: 8192, MemoryReservedMB: 2048},
		Deployments: regionstatus.DeploymentSummary{Total: 2, Pending: 1, Reserved: 1}, Tasks: regionstatus.TaskSummary{AwaitingAuthority: 1}}
	if err := db.RecordRegionStatus(ctx, "status-east", status); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRegionStatus(ctx, "status-east", status); err != nil {
		t.Fatal("idempotent replay", err)
	}
	got, err := db.GetRegionStatus(ctx, status.RegionID)
	if err != nil || got != status {
		t.Fatalf("projection: %+v %v", got, err)
	}
	conflict := status
	conflict.EventID = "different"
	if err := db.RecordRegionStatus(ctx, "status-east", conflict); !errors.Is(err, regionstatus.ErrSnapshotConflict) {
		t.Fatal("same sequence conflict accepted")
	}
	newer := status
	newer.Sequence = 2
	newer.EventID = "status-2"
	newer.ObservedAtMS = 200
	newer.Nodes.Online = 2
	if err := db.RecordRegionStatus(ctx, "status-east", newer); err != nil {
		t.Fatal(err)
	}
	older := status
	older.EventID = "older-valid"
	if err := db.RecordRegionStatus(ctx, "status-east", older); err != nil {
		t.Fatal("older delivery should be acknowledged", err)
	}
	got, _ = db.GetRegionStatus(ctx, status.RegionID)
	if got != newer {
		t.Fatal("older delivery replaced current status")
	}
	unknown := newer
	unknown.RegionID = "unknown"
	unknown.EventID = "unknown-event"
	if err := db.RecordRegionStatus(ctx, "unknown", unknown); !errors.Is(err, regionstatus.ErrSnapshotConflict) {
		t.Fatal("unknown Region accepted")
	}
}
