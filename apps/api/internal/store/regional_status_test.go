package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionstatus"
)

func testRegionalStatusPublication(t *testing.T, db *RegionalStore) {
	t.Helper()
	ctx := context.Background()
	node := regional.NodeConfiguration{ID: "status-node", Name: "Status node", Architecture: "amd64", CPU: 8, MemoryMB: 8192, Schedulable: true}
	if _, err := db.ConfigureRegionalNode(ctx, node, 0); err != nil {
		t.Fatal(err)
	}
	session, err := db.StartRegionalNodeSession(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRegionalNodeHeartbeat(ctx, node.ID, regional.NodeHeartbeat{SessionEpoch: session.Epoch, Sequence: 1, Architecture: node.Architecture, RuntimeReady: true}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CaptureRegionStatus(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RegionID != db.regionID || snapshot.Sequence != 1 || snapshot.Nodes != (regionstatus.NodeSummary{Total: 1, Online: 1, Schedulable: 1}) || snapshot.Capacity.CPUTotal != 8 || snapshot.Capacity.MemoryTotalMB != 8192 {
		t.Fatalf("status snapshot: %+v", snapshot)
	}
	messages, err := db.RegionStatusOutbox().ClaimOutbox(ctx, db.regionID, 1, time.Minute)
	if err != nil || len(messages) != 1 || messages[0].ID != snapshot.EventID {
		t.Fatalf("status outbox: %+v %v", messages, err)
	}
	var decoded regionstatus.Snapshot
	if json.Unmarshal([]byte(messages[0].Payload), &decoded) != nil || decoded != snapshot {
		t.Fatal("outbox payload differs from captured snapshot")
	}
	second, err := db.CaptureRegionStatus(ctx, time.Minute)
	if err != nil || second.Sequence != 2 || second.EventID == snapshot.EventID {
		t.Fatalf("status sequence: %+v %v", second, err)
	}
}
