package store

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/scheduling"
	"gorm.io/gorm"
)

func testRegionalCapacityCandidates(t *testing.T, db *RegionalStore, request regional.CapacityRequest) {
	ctx := context.Background()
	for _, config := range []regional.NodeConfiguration{
		{ID: "quiet", Name: "quiet", Architecture: "amd64", CPU: 2, MemoryMB: 256, Schedulable: true},
		{ID: "disabled", Name: "disabled", Architecture: "amd64", CPU: 2, MemoryMB: 256},
		{ID: "arm", Name: "arm", Architecture: "arm64", CPU: 2, MemoryMB: 256, Schedulable: true},
	} {
		if _, err := db.ConfigureRegionalNode(ctx, config, 0); err != nil {
			t.Fatal(err)
		}
		if config.ID != "quiet" {
			session, err := db.StartRegionalNodeSession(ctx, config.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.RecordRegionalNodeHeartbeat(ctx, config.ID, regional.NodeHeartbeat{SessionEpoch: session.Epoch, Sequence: 1, Architecture: config.Architecture, RuntimeReady: true}); err != nil {
				t.Fatal(err)
			}
		}
	}
	query := regional.CandidateQuery{RegionID: db.regionID, AllowedNodeIDs: []string{"node-a", "node-b", "quiet", "disabled", "arm"}, Architecture: "amd64", Resources: instances.Resources{CPU: 1, MemoryMB: 128}}
	var queries atomic.Int64
	callback := "test_regional_candidate_query_budget"
	if err := db.db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "regional_nodes", "regional_node_sessions", "regional_allocations":
			queries.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	candidates, err := db.RegionalCapacityCandidates(ctx, query, time.Minute)
	_ = db.db.Callback().Query().Remove(callback)
	if err != nil || len(candidates) != 1 || candidates[0].NodeID != "node-b" || candidates[0].NodeVersion != 1 || candidates[0].SessionEpoch != 1 || candidates[0].RemainingCPU != 0 || candidates[0].RemainingMemoryMB != 0 {
		t.Fatalf("invalid candidates: %+v %v", candidates, err)
	}
	if queries.Load() != 3 {
		t.Fatalf("candidate business query count: %d", queries.Load())
	}
	required := query
	required.RequiredNodeID = "node-a"
	if got, err := db.RegionalCapacityCandidates(ctx, required, time.Minute); err != nil || len(got) != 0 {
		t.Fatal("required unavailable node fell back")
	}
	required.RequiredNodeID = "outside"
	if _, err := db.RegionalCapacityCandidates(ctx, required, time.Minute); !errors.Is(err, regional.ErrNodeUnavailable) {
		t.Fatal("required node outside scope accepted")
	}
	scoped := query
	scoped.AllowedNodeIDs = []string{"node-a"}
	if got, err := db.RegionalCapacityCandidates(ctx, scoped, time.Minute); err != nil || len(got) != 0 {
		t.Fatal("node outside authorized page returned")
	}
	tooLarge := query
	tooLarge.Resources.CPU = 2
	if got, err := db.RegionalCapacityCandidates(ctx, tooLarge, time.Minute); err != nil || len(got) != 0 {
		t.Fatal("occupied capacity ignored")
	}
	invalid := query
	invalid.Resources.MemoryMB = 0
	if _, err := db.RegionalCapacityCandidates(ctx, invalid, time.Minute); !errors.Is(err, scheduling.ErrCapacityUnavailable) {
		t.Fatal("invalid resource request accepted")
	}
	invalid = query
	invalid.AllowedNodeIDs = make([]string, 201)
	if _, err := db.RegionalCapacityCandidates(ctx, invalid, time.Minute); !errors.Is(err, regional.ErrInvalidNode) {
		t.Fatal("unbounded candidate query accepted")
	}
	invalid = query
	invalid.AllowedNodeIDs = []string{"node-b", "node-b"}
	if _, err := db.RegionalCapacityCandidates(ctx, invalid, time.Minute); !errors.Is(err, regional.ErrInvalidNode) {
		t.Fatal("duplicate scope accepted")
	}
	invalid = query
	invalid.RegionID = "other"
	if _, err := db.RegionalCapacityCandidates(ctx, invalid, time.Minute); !errors.Is(err, ErrRegionMismatch) {
		t.Fatal("cross-region query accepted")
	}
	if err := db.db.Table("regional_node_sessions").Where("node_id = ?", "node-b").Update("last_seen_ms", int64(9223372036854775807)).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := db.RegionalCapacityCandidates(ctx, query, time.Minute); err != nil || len(got) != 0 {
		t.Fatal("future heartbeat treated as fresh")
	}
	if err := db.RecordRegionalNodeHeartbeat(ctx, "node-b", regional.NodeHeartbeat{SessionEpoch: 1, Sequence: 4, Architecture: "amd64", RuntimeReady: true}); err != nil {
		t.Fatal(err)
	}
	// A node can be cordoned after selection; transactional admission must reject
	// the previously observed version rather than trusting the candidate list.
	if _, err := db.ConfigureRegionalNode(ctx, regional.NodeConfiguration{ID: "node-b", Name: "node-b", Architecture: "amd64", CPU: 2, MemoryMB: 256}, 1); err != nil {
		t.Fatal(err)
	}
	request.NodeID = candidates[0].NodeID
	request.NodeVersion = candidates[0].NodeVersion
	request.SessionEpoch = candidates[0].SessionEpoch
	if _, err := db.ReserveRegionalCapacity(ctx, request, time.Minute); !errors.Is(err, regional.ErrNodeUnavailable) {
		t.Fatal("stale candidate admitted", err)
	}
	if got, err := db.RegionalCapacityCandidates(ctx, query, time.Minute); err != nil || len(got) != 0 {
		t.Fatal("cordoned node returned")
	}
}
