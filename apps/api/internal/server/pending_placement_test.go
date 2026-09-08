package server

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestControllersCommitInitialPlacementBeforePublishing(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "placement.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	node := domain.ComputeNode{ID: "pending-worker", CPUCores: 1, MemoryTotalMB: 1024, Status: "online", LastHeartbeat: time.Now()}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"waiting-a", "waiting-b"} {
		instance := domain.GameServer{ID: id, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}}, Status: domain.ServerRuntimeStatus{Phase: domain.PhasePending}}
		if err := db.CreateGameServer(ctx, &instance); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		controller := NewController(db, NewRuntimeReconciler(&fakeBuilder{}, nil), nil).WithScheduler(&evictionFakeScheduler{targetNode: node})
		wg.Add(1)
		go func() { defer wg.Done(); controller.RunOnce(ctx) }()
	}
	wg.Wait()
	instances, err := db.ListGameServers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assigned := 0
	for _, instance := range instances {
		assignment, err := db.GetWorkloadAssignmentByServer(ctx, instance.ID)
		if instance.NodeID == "" {
			if err != store.ErrNotFound {
				t.Fatalf("uncommitted placement published task: %+v %v", assignment, err)
			}
			if instance.Spec.Generation != 1 {
				t.Fatal("unassigned generation changed")
			}
			continue
		}
		assigned++
		if instance.NodeID != node.ID || instance.Spec.Generation != 2 || err != nil || assignment.NodeID != node.ID || assignment.Generation != 2 {
			t.Fatalf("placement/task mismatch: %+v %+v %v", instance, assignment, err)
		}
	}
	if assigned != 1 {
		t.Fatalf("assigned %d instances to one CPU", assigned)
	}
}
