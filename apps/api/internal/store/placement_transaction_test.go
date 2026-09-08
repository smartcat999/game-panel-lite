package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestPlacementTransaction(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "placement.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testPlacementTransaction(t, db)
}

func testPlacementTransaction(t *testing.T, db *Store) {
	ctx := context.Background()
	before := domain.GameServer{ID: "placement-atomic", NodeID: "source", Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped}}
	if err := db.CreateGameServer(ctx, &before); err != nil {
		t.Fatal(err)
	}
	assignment := domain.WorkloadAssignment{ID: "placement-assignment", UID: "placement-uid", ServerID: before.ID, NodeID: before.NodeID, Generation: 1, DesiredState: domain.DesiredStopped, Spec: domain.WorkloadSpec{ServerID: before.ID, Image: "image:v1"}}
	if err := db.PublishWorkloadAssignment(ctx, before, &assignment); err != nil {
		t.Fatal(err)
	}
	after := before
	after.NodeID = "target"
	after.Spec.Generation++
	assertUnchanged := func() {
		t.Helper()
		saved, err := db.GetGameServer(ctx, before.ID)
		if err != nil || saved.NodeID != before.NodeID || saved.Spec.Generation != before.Spec.Generation {
			t.Fatalf("placement mutated: %+v err=%v", saved, err)
		}
		current, err := db.GetWorkloadAssignmentByServer(ctx, before.ID)
		if err != nil || current.UID != assignment.UID {
			t.Fatalf("source assignment lost: %+v err=%v", current, err)
		}
	}
	stale := before
	stale.Spec.Generation = 0
	if err := db.MigrateGameServer(ctx, stale, after); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale migration: %v", err)
	}
	assertUnchanged()
	// Fail the second write to prove the first write rolls back too.
	createTrigger := "CREATE TRIGGER reject_placement_delete BEFORE DELETE ON workload_assignments WHEN OLD.server_id = 'placement-atomic' BEGIN SELECT RAISE(ABORT, 'injected deletion failure'); END"
	dropTrigger := "DROP TRIGGER reject_placement_delete"
	if db.db.Dialector.Name() == "postgres" {
		if err := db.db.Exec("CREATE FUNCTION reject_placement_delete_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.server_id = 'placement-atomic' THEN RAISE EXCEPTION 'injected deletion failure'; END IF; RETURN OLD; END $$").Error; err != nil {
			t.Fatal(err)
		}
		defer db.db.Exec("DROP FUNCTION reject_placement_delete_fn()")
		createTrigger = "CREATE TRIGGER reject_placement_delete BEFORE DELETE ON workload_assignments FOR EACH ROW EXECUTE FUNCTION reject_placement_delete_fn()"
		dropTrigger = "DROP TRIGGER reject_placement_delete ON workload_assignments"
	}
	if err := db.db.Exec(createTrigger).Error; err != nil {
		t.Fatal(err)
	}
	migrationErr := db.MigrateGameServer(ctx, before, after)
	if err := db.db.Exec(dropTrigger).Error; err != nil {
		t.Fatal(err)
	}
	if migrationErr == nil {
		t.Fatal("expected deletion failure")
	}
	assertUnchanged()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := after
			target.NodeID = fmt.Sprintf("target-%d", i)
			results <- db.MigrateGameServer(ctx, before, target)
		}(i)
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrReconciliationSuperseded) {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted migrations=%d want 1", accepted)
	}
	if _, err := db.GetWorkloadAssignmentByServer(ctx, before.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source assignment remains: %v", err)
	}
	saved, err := db.GetGameServer(ctx, before.ID)
	if err != nil || saved.NodeID == before.NodeID || saved.Spec.Generation != 2 {
		t.Fatalf("placement not committed: %+v err=%v", saved, err)
	}
	// A delayed source controller must not recreate the retired assignment.
	if err := db.PublishWorkloadAssignment(ctx, before, &assignment); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("source republished: %v", err)
	}
}
