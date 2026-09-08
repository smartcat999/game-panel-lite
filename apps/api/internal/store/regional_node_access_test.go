package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func testRegionalNodeAccess(t *testing.T, db *RegionalStore, create func(string) regional.CapacityRequest) {
	t.Helper()
	ctx := context.Background()
	node, err := db.ConfigureRegionalNode(ctx, regional.NodeConfiguration{ID: "access-node", Name: "access-node", Architecture: "amd64", CPU: 1, MemoryMB: 128, Schedulable: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	session, err := db.StartRegionalNodeSession(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRegionalNodeHeartbeat(ctx, node.ID, regional.NodeHeartbeat{SessionEpoch: session.Epoch, Sequence: 1, Architecture: "amd64", RuntimeReady: true}); err != nil {
		t.Fatal(err)
	}
	query := regional.CandidateQuery{OrganizationID: "unknown", RegionID: db.regionID, AllowedNodeIDs: []string{node.ID}, Architecture: "amd64", Resources: instances.Resources{CPU: 1, MemoryMB: 128}}
	if _, err := db.RegionalCapacityCandidates(ctx, query, time.Minute); !errors.Is(err, regional.ErrNodeAccessDenied) {
		t.Fatal("unconfigured tenant admitted", err)
	}
	query.OrganizationID = "tenant"
	if found, err := db.RegionalCapacityCandidates(ctx, query, time.Minute); err != nil || len(found) != 0 {
		t.Fatal("candidate scope bypassed policy", err)
	}
	pinned := query
	pinned.RequiredNodeID = node.ID
	if _, err := db.RegionalCapacityCandidates(ctx, pinned, time.Minute); !errors.Is(err, regional.ErrNodeAccessDenied) {
		t.Fatal("pin bypassed policy", err)
	}
	request := create("node-access-server")
	request.NodeID = node.ID
	if _, err := db.ReserveRegionalResources(ctx, request, workload.Network{}, time.Minute); !errors.Is(err, regional.ErrNodeAccessDenied) {
		t.Fatal("direct reservation bypassed policy", err)
	}
	policy := regional.NodeAccessPolicy{OrganizationID: "tenant", NodeIDs: []string{node.ID}, Enabled: true}
	updated, err := db.ConfigureRegionalNodeAccess(ctx, policy, 1)
	if err != nil || updated.Version != 2 {
		t.Fatal("policy update", err)
	}
	if _, err := db.ConfigureRegionalNodeAccess(ctx, policy, 1); !errors.Is(err, regional.ErrNodeAccessVersionConflict) {
		t.Fatal("stale policy overwrote", err)
	}
	if found, err := db.RegionalCapacityCandidates(ctx, query, time.Minute); err != nil || len(found) != 1 {
		t.Fatal("grant not applied", err)
	}
	// Revoke after candidate observation; transactional admission must recheck.
	policy.Enabled = false
	if _, err := db.ConfigureRegionalNodeAccess(ctx, policy, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveRegionalResources(ctx, request, workload.Network{}, time.Minute); !errors.Is(err, regional.ErrNodeAccessDenied) {
		t.Fatal("stale candidate granted access", err)
	}
	policy.Enabled = true
	if _, err := db.ConfigureRegionalNodeAccess(ctx, policy, 3); err != nil {
		t.Fatal(err)
	}
	allocation, err := db.ReserveRegionalResources(ctx, request, workload.Network{}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	policy.Enabled = false
	if _, err := db.ConfigureRegionalNodeAccess(ctx, policy, 4); err != nil {
		t.Fatal(err)
	}
	replay, err := db.ReserveRegionalResources(ctx, request, workload.Network{}, time.Minute)
	if err != nil || !reflect.DeepEqual(replay, allocation) {
		t.Fatal("revocation destroyed historical receipt", err)
	}
	// Policy changes never assert physical stop or release reserved resources.
	var count int64
	if err := db.db.Table("regional_allocations").Where("id = ? AND status = ?", allocation.ID, "reserved").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("policy revocation released resource", err)
	}
	for _, invalid := range []regional.NodeAccessPolicy{{OrganizationID: ""}, {OrganizationID: "tenant", NodeIDs: []string{"a", "a"}}, {OrganizationID: "tenant", NodeIDs: []string{" bad"}}} {
		if _, err := db.ConfigureRegionalNodeAccess(ctx, invalid, 5); !errors.Is(err, regional.ErrNodeAccessDenied) {
			t.Fatal("invalid policy accepted", err)
		}
	}
	t.Log("tenant node policy: default deny, filtering, strict pin, CAS, revocation before admission and retained receipts verified")
}
