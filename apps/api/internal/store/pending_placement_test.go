package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestPendingPlacement(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pending.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testPendingPlacement(t, db)
}

func testPendingPlacement(t *testing.T, db *Store) {
	ctx := context.Background()
	node := domain.ComputeNode{ID: "pending-target", CPUCores: 2, MemoryTotalMB: 1024}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	pending := func(id string) domain.GameServer {
		return domain.GameServer{ID: id, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}}}
	}
	var candidates []domain.GameServer
	for i := 0; i < 8; i++ {
		before := pending(fmt.Sprintf("pending-candidate-%d", i))
		if err := db.CreateGameServer(ctx, &before); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, before)
	}
	results := make(chan error, 8)
	for _, before := range candidates {
		go func(before domain.GameServer) {
			after := before
			after.NodeID = node.ID
			after.Spec.Generation++
			results <- db.AssignPendingGameServer(ctx, before, after)
		}(before)
	}
	accepted := 0
	for i := 0; i < 8; i++ {
		err := <-results
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrNodeAllocationUnavailable) {
			t.Fatal(err)
		}
	}
	if accepted != 2 {
		t.Fatalf("allocated %d want 2", accepted)
	}
	for _, before := range candidates {
		saved, err := db.GetGameServer(ctx, before.ID)
		if err != nil {
			t.Fatal(err)
		}
		if saved.NodeID == "" {
			if saved.Spec.Generation != 1 {
				t.Fatal("failed allocation changed generation")
			}
			continue
		}
		if saved.NodeID != node.ID || saved.Spec.Generation != 2 {
			t.Fatalf("placement not persisted: %+v", saved)
		}
		after := before
		after.NodeID = node.ID
		after.Spec.Generation++
		if err := db.AssignPendingGameServer(ctx, before, after); !errors.Is(err, ErrReconciliationSuperseded) {
			t.Fatalf("stale allocator accepted: %v", err)
		}
	}
	orphan := pending("pending-orphan")
	if err := db.CreateGameServer(ctx, &orphan); err != nil {
		t.Fatal(err)
	}
	recoveryNode := domain.ComputeNode{ID: "pending-recovery-target", CPUCores: 8, MemoryTotalMB: 8192}
	if err := db.CreateComputeNode(ctx, &recoveryNode); err != nil {
		t.Fatal(err)
	}
	old := domain.WorkloadAssignment{ID: "orphan-assignment", UID: "orphan-uid", ServerID: orphan.ID, NodeID: "old-node", Generation: 1}
	if err := db.db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	after := orphan
	after.NodeID = recoveryNode.ID
	after.Spec.Generation++
	if err := db.AssignPendingGameServer(ctx, orphan, after); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("orphan workload treated as fresh: %v", err)
	}
	saved, err := db.GetGameServer(ctx, orphan.ID)
	if err != nil || saved.NodeID != "" || saved.Spec.Generation != 1 {
		t.Fatalf("orphan rejection not atomic: %+v %v", saved, err)
	}
	retained, err := db.GetWorkloadAssignmentByServer(ctx, orphan.ID)
	if err != nil || retained.UID != old.UID {
		t.Fatalf("lost recovery evidence: %+v %v", retained, err)
	}
	after.Spec.Resources.CPULimitCores = 2
	if err := db.AssignPendingGameServer(ctx, orphan, after); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("placement modified resource intent: %v", err)
	}
	// Retired lease history also proves this is not a first placement, even after
	// the old assignment was removed. Expiry alone is not source-process fencing.
	if err := db.DeleteWorkloadAssignment(ctx, orphan.ID); err != nil {
		t.Fatal(err)
	}
	lease := ExecutionLease{ServerID: orphan.ID, AssignmentUID: old.UID, NodeID: old.NodeID, Generation: 1, HolderID: "old-holder", Fence: 1}
	if err := db.db.Create(&lease).Error; err != nil {
		t.Fatal(err)
	}
	after.Spec = orphan.Spec
	after.Spec.Generation++
	if err := db.AssignPendingGameServer(ctx, orphan, after); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("retired lease treated as fresh work: %v", err)
	}
	saved, err = db.GetGameServer(ctx, orphan.ID)
	if err != nil || saved.NodeID != "" || saved.Spec.Generation != 1 {
		t.Fatalf("lease-history rejection not atomic: %+v %v", saved, err)
	}

}
