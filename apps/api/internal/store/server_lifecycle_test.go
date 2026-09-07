package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestServerLifecycleWrites(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "lifecycle.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testServerLifecycleWrites(t, db)
}

func testServerLifecycleWrites(t *testing.T, db *Store) {
	ctx := context.Background()
	org := domain.Organization{ID: "lifecycle-org", Slug: "lifecycle-org"}
	if err := db.CreateOrganization(ctx, &org, "lifecycle-owner"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"config", "status", "node", "organization", "deleted", "metadata", "unchanged", "concurrent"} {
		t.Run("lifecycle_"+change, func(t *testing.T) {
			before := domain.GameServer{ID: "lifecycle-" + change, OrganizationID: org.ID, Name: "original", Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1024}}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
			if err := db.CreateGameServer(ctx, &before); err != nil {
				t.Fatal(err)
			}
			current := before
			switch change {
			case "config":
				current.Spec.Generation++
				current.Spec.Resources.MemoryLimitMB = 2048
			case "status":
				current.Status.Phase = domain.PhaseRunning
			case "node":
				current.NodeID = "another-node"
			case "organization":
				current.OrganizationID = "another-org"
			case "metadata":
				current.Name = "renamed"
			}
			if change == "deleted" {
				if err := db.DeleteGameServer(ctx, before.ID); err != nil {
					t.Fatal(err)
				}
			} else if err := db.db.WithContext(ctx).Save(&current).Error; err != nil {
				t.Fatal(err)
			}
			after := before
			after.Spec.DesiredState = domain.DesiredDeleted
			after.Spec.Generation++
			after.Status.Phase = domain.PhaseDeleting
			// Lifecycle callers have no authority to alter these fields.
			after.Spec.Resources.MemoryLimitMB = 99999
			after.Status.PlayersOnline = 999
			after.Name = "untrusted"
			n := 1
			if change == "concurrent" {
				n = 8
			}
			results := make(chan error, n)
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); results <- db.SaveServerLifecycle(ctx, "lifecycle-owner", before, after) }()
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
			want := 0
			if change == "metadata" || change == "unchanged" || change == "concurrent" {
				want = 1
			}
			if accepted != want {
				t.Fatalf("accepted %d transitions, want %d", accepted, want)
			}
			got, err := db.GetGameServer(ctx, before.ID)
			if change == "deleted" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("resurrected instance: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Spec.Resources != current.Spec.Resources || got.Name != current.Name || got.Status.PlayersOnline != current.Status.PlayersOnline || got.OrganizationID != current.OrganizationID || got.NodeID != current.NodeID {
				t.Fatalf("unowned fields overwritten: %+v", got)
			}
			expectedPhase := current.Status.Phase
			if want == 1 {
				expectedPhase = domain.PhaseDeleting
			}
			if got.Status.Phase != expectedPhase {
				t.Fatalf("phase: %s", got.Status.Phase)
			}
		})
	}
	before, err := db.GetGameServer(ctx, "lifecycle-unchanged")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RemoveOrganizationMember(ctx, org.ID, "lifecycle-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveServerLifecycle(ctx, "lifecycle-owner", before, before); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("revoked writer: %v", err)
	}
}
