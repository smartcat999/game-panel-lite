package store

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
)

func testRegionalPorts(t *testing.T, db *RegionalStore, create func(string) regional.CapacityRequest) {
	ctx := context.Background()
	for _, id := range []string{"port-node", "free-port-node"} {
		if _, err := db.ConfigureRegionalNode(ctx, regional.NodeConfiguration{ID: id, Name: id, Architecture: "amd64", CPU: 8, MemoryMB: 1024, Schedulable: true}, 0); err != nil {
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
	request := func(id, node string) regional.CapacityRequest { r := create(id); r.NodeID = node; return r }
	first := request("port-first", "port-node")
	network := workload.Network{Port: 7777, HostPort: 20000, AdditionalPorts: []workload.Port{{Port: 123, HostPort: 20001, Protocol: "udp"}}}
	allocation, err := db.ReserveRegionalResources(ctx, first, network, time.Minute)
	if err != nil || len(allocation.Ports) != 2 {
		t.Fatal("complete network not reserved", err)
	}
	equivalent := workload.Network{AdditionalPorts: []workload.Port{{Port: 123, HostPort: 20001, Protocol: "udp"}, {Port: 7777, HostPort: 20000, Protocol: "tcp"}, {Port: 7777, HostPort: 20000}}}
	replay, err := db.ReserveRegionalResources(ctx, first, equivalent, time.Minute)
	if err != nil || !reflect.DeepEqual(replay, allocation) {
		t.Fatal("normalized replay changed ports", err)
	}
	changed := network
	changed.HostPort = 20100
	if _, err := db.ReserveRegionalResources(ctx, first, changed, time.Minute); !errors.Is(err, regional.ErrAllocationConflict) {
		t.Fatal("replay changed network")
	}
	blocked := request("port-blocked", "port-node")
	if _, err := db.ReserveRegionalResources(ctx, blocked, workload.Network{Port: 80, HostPort: 20002, AdditionalPorts: []workload.Port{{Port: 123, HostPort: 20001, Protocol: "udp"}}}, time.Minute); !errors.Is(err, regional.ErrPortsUnavailable) {
		t.Fatal("additional port conflict accepted", err)
	}
	var count int64
	if err := db.db.Table("regional_allocations").Where("node_id = ?", "port-node").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("port rejection left compute reservation")
	}
	udp := request("port-udp", "port-node")
	if _, err := db.ReserveRegionalResources(ctx, udp, workload.Network{Port: 7777, HostPort: 20000, Protocol: "udp"}, time.Minute); err != nil {
		t.Fatal("TCP and UDP incorrectly conflict", err)
	}
	invalid := network
	invalid.AdditionalPorts = append(invalid.AdditionalPorts, workload.Port{Port: 9999, HostPort: 20000, Protocol: "tcp"})
	if _, err := db.ReserveRegionalResources(ctx, blocked, invalid, time.Minute); !errors.Is(err, workload.ErrInvalidNetwork) {
		t.Fatal("ambiguous host binding accepted")
	}
	query := regional.CandidateQuery{RegionID: db.regionID, AllowedNodeIDs: []string{"port-node", "free-port-node"}, Architecture: "amd64", Resources: instances.Resources{CPU: 1, MemoryMB: 128}, Network: network}
	var queries atomic.Int64
	callback := "test_regional_ports_query_budget"
	if err := db.db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "regional_nodes", "regional_node_sessions", "regional_allocations", "regional_port_reservations":
			queries.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	candidates, err := db.RegionalCapacityCandidates(ctx, query, time.Minute)
	_ = db.db.Callback().Query().Remove(callback)
	if err != nil || len(candidates) != 1 || candidates[0].NodeID != "free-port-node" || queries.Load() != 4 {
		t.Fatalf("port candidates %+v queries=%d err=%v", candidates, queries.Load(), err)
	}
	query.RequiredNodeID = "port-node"
	if got, err := db.RegionalCapacityCandidates(ctx, query, time.Minute); err != nil || len(got) != 0 {
		t.Fatal("port conflict fell back from required node")
	}
	other := request("port-other", "free-port-node")
	if _, err := db.ReserveRegionalResources(ctx, other, network, time.Minute); err != nil {
		t.Fatal("same ports on another node rejected", err)
	}
	stale := request("port-stale", "free-port-node")
	if _, err := db.ReserveRegionalResources(ctx, stale, network, time.Minute); !errors.Is(err, regional.ErrPortsUnavailable) {
		t.Fatal("stale port candidate admitted")
	}
	failed := request("port-failed", "port-node")
	free := workload.Network{Port: 30000}
	if err := db.db.Exec("ALTER TABLE regional_port_reservations ADD CONSTRAINT test_port_failure CHECK(false) NOT VALID").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveRegionalResources(ctx, failed, free, time.Minute); err == nil {
		t.Fatal("port persistence failure ignored")
	}
	if err := db.db.Table("regional_allocations").Where("deployment_id = ?", failed.DeploymentID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("port insert failure retained compute")
	}
	if err := db.db.Exec("ALTER TABLE regional_port_reservations DROP CONSTRAINT test_port_failure").Error; err != nil {
		t.Fatal(err)
	}
	complete, err := db.ReserveRegionalResources(ctx, failed, free, time.Minute)
	if err != nil || complete.Ports[0].HostPort != 30000 {
		t.Fatal("default host port differs from runtime", err)
	}
	// Competing deployments must not both acquire the same host binding.
	raceRequests := []regional.CapacityRequest{request("port-race-a", "port-node"), request("port-race-b", "port-node")}
	var wg sync.WaitGroup
	raceResults := make(chan error, 2)
	for _, r := range raceRequests {
		wg.Add(1)
		go func(r regional.CapacityRequest) {
			defer wg.Done()
			_, err := db.ReserveRegionalResources(ctx, r, workload.Network{Port: 40000}, time.Minute)
			raceResults <- err
		}(r)
	}
	wg.Wait()
	close(raceResults)
	successes := 0
	for err := range raceResults {
		if err == nil {
			successes++
		} else if !errors.Is(err, regional.ErrPortsUnavailable) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("concurrent port binding was not unique")
	}
	// Corrupt only the test receipt to verify retries cannot hide missing bindings.
	if err := db.db.Table("regional_port_reservations").Where("allocation_id = ? AND host_port = ?", allocation.ID, 20001).Delete(&regionalPortRow{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveRegionalResources(ctx, first, network, time.Minute); !errors.Is(err, regional.ErrAllocationConflict) {
		t.Fatal("incomplete persisted port receipt accepted")
	}
}
