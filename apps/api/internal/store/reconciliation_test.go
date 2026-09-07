package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestReconciliationPersistence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "reconciliation.db"))
	if err != nil {
		t.Fatal(err)
	}
	testReconciliationPersistence(t, db)
}

func testReconciliationPersistence(t *testing.T, db *Store) {
	ctx := context.Background()
	for _, change := range []string{"resources", "same-generation", "node", "organization", "deleted", "name", "unchanged"} {
		t.Run("reconcile_"+change, func(t *testing.T) {
			original := domain.GameServer{ID: "reconcile-" + change, Name: "original", OrganizationID: "workspace", NodeID: "node", Spec: domain.ServerSpec{Generation: 1, Runtime: domain.ServerRuntimeSpec{ModSyncMode: "sync"}}, Status: domain.ServerRuntimeStatus{Phase: domain.PhasePending}}
			if err := db.CreateGameServer(ctx, &original); err != nil {
				t.Fatal(err)
			}
			current := original
			switch change {
			case "resources":
				current.Spec.Generation++
				current.Spec.Resources.MemoryLimitMB = 4096
			case "same-generation":
				current.Spec.Resources.MemoryLimitMB = 4096
			case "node":
				current.NodeID = "another-node"
			case "organization":
				current.OrganizationID = "another-workspace"
			case "name":
				current.Name = "renamed"
			}
			if change == "deleted" {
				if err := db.DeleteGameServer(ctx, original.ID); err != nil {
					t.Fatal(err)
				}
			} else if err := db.db.WithContext(ctx).Save(&current).Error; err != nil {
				t.Fatal(err)
			}
			result := original
			result.Status.Phase = domain.PhaseRunning
			result.Spec.Runtime.ModSyncMode = ""
			// Results have no authority to change resource reservations or names.
			result.Spec.Resources.MemoryLimitMB = 99999
			result.Name = "stale-name"
			err := db.SaveReconciledGameServer(ctx, original, result)
			accepted := change == "unchanged" || change == "name"
			if accepted && err != nil {
				t.Fatal(err)
			}
			if !accepted && !errors.Is(err, ErrReconciliationSuperseded) {
				t.Fatalf("expected superseded, got %v", err)
			}
			stored, err := db.GetGameServer(ctx, original.ID)
			if change == "deleted" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("deleted instance resurrected: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if stored.Name != current.Name || stored.NodeID != current.NodeID || stored.OrganizationID != current.OrganizationID || stored.Spec.Resources != current.Spec.Resources {
				t.Fatalf("user intent overwritten: %+v", stored)
			}
			if accepted {
				if stored.Status.Phase != domain.PhaseRunning || stored.Spec.Runtime.ModSyncMode != "" {
					t.Fatalf("result not persisted: %+v", stored)
				}
			} else if stored.Status.Phase != domain.PhasePending || stored.Spec.Runtime.ModSyncMode != "sync" {
				t.Fatalf("stale result persisted: %+v", stored)
			}
		})
	}
}
