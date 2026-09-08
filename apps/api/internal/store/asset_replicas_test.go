package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func testAssetReplicas(t *testing.T, db *Store, event instances.RevisionAvailable, ref instances.AssetVersion) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{"replica-east", "replica-west"} {
		if _, err := db.RegisterRegion(ctx, id, id); err != nil {
			t.Fatal(err)
		}
	}
	replica := assets.Replica{ID: "replica-a", AssetID: ref.AssetID, AssetVersion: ref.Version, RegionID: "replica-east", StorageID: "storage-one"}
	created, err := db.RegisterAssetReplica(ctx, replica)
	if err != nil || created.Available || created.Version != 1 {
		t.Fatalf("replica registration: %+v %v", created, err)
	}
	if rows, err := db.ListRegionalAssetSources(ctx, event.RegionID, event, ref, "", 10); err != nil || len(rows) != 0 {
		t.Fatal("unverified registration became a source")
	}
	if err := db.SetAssetReplicaAvailable(ctx, "replica-west", replica.ID, 1, true); !errors.Is(err, assets.ErrReplicaConflict) {
		t.Fatal("other region updated source")
	}
	if err := db.SetAssetReplicaAvailable(ctx, "replica-east", replica.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAssetReplicaAvailable(ctx, "replica-east", replica.ID, 1, false); !errors.Is(err, assets.ErrReplicaConflict) {
		t.Fatal("stale replica observation accepted")
	}
	if repeated, err := db.RegisterAssetReplica(ctx, replica); err != nil || !repeated.Available || repeated.Version != 2 {
		t.Fatal("registration reset observation")
	}
	duplicate := replica
	duplicate.ID = "duplicate-location"
	if _, err := db.RegisterAssetReplica(ctx, duplicate); !errors.Is(err, assets.ErrReplicaConflict) {
		t.Fatal("same location registered with another identity")
	}
	changed := replica
	changed.StorageID = "another-store"
	if _, err := db.RegisterAssetReplica(ctx, changed); !errors.Is(err, assets.ErrReplicaConflict) {
		t.Fatal("replica rebound to another storage")
	}
	missing := replica
	missing.ID = "missing-version"
	missing.AssetVersion = "absent"
	if _, err := db.RegisterAssetReplica(ctx, missing); !errors.Is(err, assets.ErrUnavailable) {
		t.Fatal("replica registered for missing asset version")
	}
	second := replica
	second.ID = "replica-b"
	second.RegionID = "replica-west"
	if _, err := db.RegisterAssetReplica(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAssetReplicaAvailable(ctx, second.RegionID, second.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	first, err := db.ListRegionalAssetSources(ctx, event.RegionID, event, ref, "", 1)
	if err != nil || len(first) != 1 || first[0].ID != replica.ID {
		t.Fatalf("source first page: %+v %v", first, err)
	}
	next, err := db.ListRegionalAssetSources(ctx, event.RegionID, event, ref, first[0].ID, 1)
	if err != nil || len(next) != 1 || next[0].ID != second.ID {
		t.Fatalf("source next page: %+v %v", next, err)
	}
	if rows, err := db.ListRegionalAssetSources(ctx, "wrong-region", event, ref, "", 10); !errors.Is(err, ErrNotFound) || rows != nil {
		t.Fatal("source directory disclosed across regions")
	}
	if rows, err := db.ListRegionalAssetSources(ctx, event.RegionID, event, instances.AssetVersion{AssetID: ref.AssetID, Version: "absent"}, "", 10); !errors.Is(err, ErrNotFound) || rows != nil {
		t.Fatal("unreferenced version listed sources")
	}
	if db.db.Dialector.Name() == "postgres" {
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for _, available := range []bool{true, false} {
			wg.Add(1)
			go func(a bool) {
				defer wg.Done()
				results <- db.SetAssetReplicaAvailable(ctx, replica.RegionID, replica.ID, 2, a)
			}(available)
		}
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			} else if !errors.Is(err, assets.ErrReplicaConflict) {
				t.Fatal(err)
			}
		}
		if success != 1 {
			t.Fatal("replica CAS accepted two observations")
		}
		if err := db.SetAssetReplicaAvailable(ctx, replica.RegionID, replica.ID, 3, false); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := db.SetAssetReplicaAvailable(ctx, replica.RegionID, replica.ID, 2, false); err != nil {
			t.Fatal(err)
		}
		if err := migrateSQLiteAssetReplicas(db.db); err != nil {
			t.Fatal(err)
		}
	}
	if rows, err := db.ListRegionalAssetSources(ctx, event.RegionID, event, ref, "", 10); err != nil || len(rows) != 1 || rows[0].ID != second.ID {
		t.Fatal("unavailable source still listed")
	}
}
