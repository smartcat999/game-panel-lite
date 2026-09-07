package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestPlayerObservations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "players.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testPlayerObservations(t, db)
}

func testPlayerObservations(t *testing.T, db *Store) {
	ctx := context.Background()
	for _, change := range []string{"config", "status", "runtime", "node", "organization", "deleted", "unchanged"} {
		t.Run("player_"+change, func(t *testing.T) {
			before := domain.GameServer{ID: "player-" + change, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseRunning, RuntimeID: "runtime", PlayersOnline: 1}}
			if err := db.CreateGameServer(ctx, &before); err != nil {
				t.Fatal(err)
			}
			current := before
			switch change {
			case "config":
				current.Name = "renamed"
				current.Spec.Generation = 2
				current.Spec.Resources.MemoryLimitMB = 2048
			case "status":
				current.Status.Phase = domain.PhaseStopped
				current.Status.PlayersOnline = 0
			case "runtime":
				current.Status.RuntimeID = "replacement"
			case "node":
				current.NodeID = "another-node"
			case "organization":
				current.OrganizationID = "another-workspace"
			}
			if change == "deleted" {
				if err := db.DeleteGameServer(ctx, before.ID); err != nil {
					t.Fatal(err)
				}
			} else if err := db.db.WithContext(ctx).Save(&current).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.SavePlayerCount(ctx, before, 8); err != nil {
				t.Fatal(err)
			}
			got, err := db.GetGameServer(ctx, before.ID)
			if change == "deleted" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("deleted instance resurrected: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantCount := current.Status.PlayersOnline
			if change == "config" || change == "unchanged" {
				wantCount = 8
			}
			if got.Status.PlayersOnline != wantCount || got.Status.Phase != current.Status.Phase || got.Status.RuntimeID != current.Status.RuntimeID || got.NodeID != current.NodeID || got.OrganizationID != current.OrganizationID || got.Name != current.Name || got.Spec.Generation != current.Spec.Generation || got.Spec.Resources != current.Spec.Resources {
				t.Fatalf("observation overwrote current instance: %+v", got)
			}
		})
	}
}
