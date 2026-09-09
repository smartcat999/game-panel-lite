package store

import (
	"context"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func testRegionalNodeOperations(t *testing.T, db *RegionalStore) {
	t.Helper()
	ctx := context.Background()
	for _, config := range []regional.NodeConfiguration{
		{ID: "ops-a", Name: "Operations A", Architecture: "amd64", CPU: 8, MemoryMB: 8192, Schedulable: true},
		{ID: "ops-b", Name: "Operations B", Architecture: "arm64", CPU: 4, MemoryMB: 4096, Schedulable: false},
	} {
		if _, err := db.ConfigureRegionalNode(ctx, config, 0); err != nil {
			t.Fatal(err)
		}
	}
	session, err := db.StartRegionalNodeSession(ctx, "ops-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRegionalNodeHeartbeat(ctx, "ops-a", regional.NodeHeartbeat{SessionEpoch: session.Epoch, Sequence: 1, Architecture: "amd64", RuntimeReady: true}); err != nil {
		t.Fatal(err)
	}
	page, err := db.ListRegionalNodeOperations(ctx, "", 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if page.RegionID != db.regionID || page.ObservedAtMS < 1 || len(page.Nodes) != 1 || page.Nodes[0].ID != "ops-a" || !page.Nodes[0].Online || page.NextCursor != "ops-a" {
		t.Fatalf("first node operations page: %+v", page)
	}
	page, err = db.ListRegionalNodeOperations(ctx, "ops-a", 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Nodes) != 1 || page.Nodes[0].ID != "ops-b" || page.Nodes[0].Online || page.NextCursor != "ops-b" {
		t.Fatalf("filtered node operations page: %+v", page)
	}
}
