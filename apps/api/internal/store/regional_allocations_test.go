package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/scheduling"
)

func testRegionalAllocations(t *testing.T, db *RegionalStore, dsn string) {
	ctx := context.Background()
	configure := func(id string) {
		t.Helper()
		if _, err := db.ConfigureRegionalNode(ctx, regional.NodeConfiguration{ID: id, Name: id, Architecture: "amd64", CPU: 2, MemoryMB: 256, Schedulable: true}, 0); err != nil {
			t.Fatal(err)
		}
		session, err := db.StartRegionalNodeSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.RecordRegionalNodeHeartbeat(ctx, id, regional.NodeHeartbeat{SessionEpoch: session.Epoch, Sequence: 1, Architecture: "amd64", RuntimeReady: true}); err != nil {
			t.Fatal(err)
		}
	}
	configure("node-a")
	configure("node-b")
	create := func(id string) regional.CapacityRequest {
		t.Helper()
		event := instances.RevisionAvailable{SchemaVersion: 1, EventID: id, OperationID: id, OrganizationID: "tenant", ServerID: id, RegionID: db.regionID, RevisionID: id, SpecGeneration: 1, PlacementEpoch: 1}
		if err := db.RecordRevisionNotification(ctx, event); err != nil {
			t.Fatal(err)
		}
		fetch, err := db.ClaimRevision(ctx, time.Minute)
		if err != nil || fetch == nil {
			t.Fatal("claim revision", err)
		}
		snapshot := regional.RevisionSnapshot{Event: event, CurrentSpecGeneration: 1, IntentVersion: 1, DesiredState: "running", Revision: instances.Revision{ID: id, ServerID: id, SpecGeneration: 1, Specification: instances.Specification{ProviderKey: "fixture", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 128}, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("protected")}}}}
		if err := db.SaveRevision(ctx, *fetch, snapshot); err != nil {
			t.Fatal(err)
		}
		assets, err := db.ClaimAssets(ctx, time.Minute)
		if err != nil || assets == nil {
			t.Fatal("claim assets", err)
		}
		if err := db.CompleteAssets(ctx, *assets); err != nil {
			t.Fatal(err)
		}
		var d regional.Deployment
		if err := db.db.Table("regional_deployments").Where("server_id = ?", id).Take(&d).Error; err != nil {
			t.Fatal(err)
		}
		return regional.CapacityRequest{RegionID: db.regionID, OrganizationID: "tenant", DeploymentID: d.ID, ServerID: id, PlacementEpoch: 1, RevisionID: id, SpecGeneration: 1, IntentVersion: 1, NodeID: "node-a", NodeVersion: 1, SessionEpoch: 1}
	}
	requests := make([]regional.CapacityRequest, 8)
	for i := range requests {
		requests[i] = create(fmt.Sprintf("server-%d", i))
	}
	type outcome struct {
		allocation regional.Allocation
		err        error
	}
	results := make(chan outcome, 8)
	var wg sync.WaitGroup
	for _, r := range requests {
		wg.Add(1)
		go func(r regional.CapacityRequest) {
			defer wg.Done()
			a, err := db.ReserveRegionalResources(ctx, r, workload.Network{}, time.Minute)
			results <- outcome{a, err}
		}(r)
	}
	wg.Wait()
	close(results)
	held := []regional.Allocation{}
	for result := range results {
		if result.err == nil {
			held = append(held, result.allocation)
		} else if !errors.Is(result.err, scheduling.ErrCapacityUnavailable) {
			t.Fatal(result.err)
		}
	}
	if len(held) != 2 {
		t.Fatalf("concurrent capacity reservations: %d", len(held))
	}
	replay, err := db.ReserveRegionalResources(ctx, held[0].CapacityRequest, workload.Network{}, time.Minute)
	if err != nil || !reflect.DeepEqual(replay, held[0]) {
		t.Fatal("reservation replay changed identity", err)
	}
	changed := held[0].CapacityRequest
	changed.NodeID = "node-b"
	if _, err := db.ReserveRegionalResources(ctx, changed, workload.Network{}, time.Minute); !errors.Is(err, regional.ErrAllocationConflict) {
		t.Fatal("existing deployment silently changed node")
	}
	request := create("pending")
	request.NodeID = "node-b"
	for _, change := range []func(*regional.CapacityRequest){func(r *regional.CapacityRequest) { r.OrganizationID = "other" }, func(r *regional.CapacityRequest) { r.SpecGeneration++ }, func(r *regional.CapacityRequest) { r.IntentVersion++ }, func(r *regional.CapacityRequest) { r.PlacementEpoch++ }} {
		wrong := request
		change(&wrong)
		if _, err := db.ReserveRegionalResources(ctx, wrong, workload.Network{}, time.Minute); !errors.Is(err, regional.ErrDeploymentConflict) {
			t.Fatal("stale or foreign deployment accepted", err)
		}
	}
	wrong := request
	wrong.RegionID = "other"
	if _, err := db.ReserveRegionalResources(ctx, wrong, workload.Network{}, time.Minute); !errors.Is(err, ErrRegionMismatch) {
		t.Fatal("foreign region accepted")
	}
	for _, change := range []func(*regional.CapacityRequest){func(r *regional.CapacityRequest) { r.NodeVersion++ }, func(r *regional.CapacityRequest) { r.SessionEpoch++ }} {
		wrong := request
		change(&wrong)
		if _, err := db.ReserveRegionalResources(ctx, wrong, workload.Network{}, time.Minute); !errors.Is(err, regional.ErrNodeUnavailable) {
			t.Fatal("stale node observation accepted", err)
		}
	}
	if err := db.db.Table("regional_node_sessions").Where("node_id = ?", "node-b").Update("last_seen_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveRegionalResources(ctx, request, workload.Network{}, time.Minute); !errors.Is(err, regional.ErrNodeUnavailable) {
		t.Fatal("stale heartbeat admitted", err)
	}
	if err := db.RecordRegionalNodeHeartbeat(ctx, "node-b", regional.NodeHeartbeat{SessionEpoch: 1, Sequence: 2, Architecture: "amd64", RuntimeReady: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveRegionalResources(ctx, request, workload.Network{}, time.Minute); !errors.Is(err, regional.ErrNodeUnavailable) {
		t.Fatal("unready runtime admitted")
	}
	if err := db.RecordRegionalNodeHeartbeat(ctx, "node-b", regional.NodeHeartbeat{SessionEpoch: 1, Sequence: 3, Architecture: "amd64", RuntimeReady: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.db.Exec("ALTER TABLE regional_allocations ADD CONSTRAINT test_allocation_failure CHECK(false) NOT VALID").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveRegionalResources(ctx, request, workload.Network{}, time.Minute); err == nil {
		t.Fatal("allocation persistence failure ignored")
	}
	var count int64
	if err := db.db.Table("regional_allocations").Count(&count).Error; err != nil || count != 2 {
		t.Fatal("partial reservation escaped rollback")
	}
	if err := db.db.Exec("ALTER TABLE regional_allocations DROP CONSTRAINT test_allocation_failure").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveRegionalResources(ctx, request, workload.Network{}, time.Minute); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenRegionalPostgres(dsn, db.regionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := db.db.Table("regional_node_sessions").Where("node_id = ?", "node-a").Update("last_seen_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.ReserveRegionalResources(ctx, held[0].CapacityRequest, workload.Network{}, time.Minute); err != nil || !reflect.DeepEqual(got, held[0]) {
		t.Fatal("receipt replay renewed or lost reservation")
	}
	if err := db.db.Table("regional_allocations").Where("status = ?", "reserved").Count(&count).Error; err != nil || count != 3 {
		t.Fatal("offline observation released capacity")
	}
	testRegionalCapacityCandidates(t, db, create("candidate"))
	stopped := create("stopped")
	stopped.NodeID = "node-b"
	if err := db.db.Table("regional_deployments").Where("id = ?", stopped.DeploymentID).Update("desired_state", "stopped").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveRegionalResources(ctx, stopped, workload.Network{}, time.Minute); !errors.Is(err, regional.ErrDeploymentConflict) {
		t.Fatal("stopped intent reserved compute")
	}
	testRegionalPorts(t, db, create)
	testRegionalScheduler(t, db)
}
