package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
)

func testRegionDirectory(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{"catalog-c", "catalog-a", "catalog-b"} {
		entry, err := db.RegisterRegion(ctx, id, id)
		if err != nil || entry.AcceptingCreates || entry.Version != 1 {
			t.Fatalf("region registration: %+v %v", entry, err)
		}
	}
	if _, err := db.RegisterRegion(ctx, "catalog-a", "different name"); !errors.Is(err, regions.ErrRegionConflict) {
		t.Fatalf("duplicate replaced region: %v", err)
	}
	if _, err := db.RegisterRegion(ctx, "Bad Region", "invalid"); !errors.Is(err, regions.ErrInvalidRegion) {
		t.Fatal("invalid identity accepted")
	}
	first, err := db.ListRegions(ctx, "", 2)
	if err != nil || len(first) != 2 || first[0].ID != "catalog-a" || first[1].ID != "catalog-b" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	second, err := db.ListRegions(ctx, first[1].ID, 1)
	if err != nil || len(second) != 1 || second[0].ID != "catalog-c" {
		t.Fatalf("second page: %+v %v", second, err)
	}
	if err := db.SetRegionAcceptingCreates(ctx, "catalog-a", 1, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetRegionAcceptingCreates(ctx, "catalog-a", 1, false); !errors.Is(err, regions.ErrRegionConflict) {
		t.Fatalf("stale decision accepted: %v", err)
	}
	entry, err := db.GetRegion(ctx, "catalog-a")
	if err != nil || !entry.AcceptingCreates || entry.Version != 2 || entry.Name != "catalog-a" {
		t.Fatalf("catalog state: %+v %v", entry, err)
	}
	if err := db.SetRegionAcceptingCreates(ctx, "catalog-a", 2, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetRegion(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing region: %v", err)
	}
	if _, err := db.ListRegions(ctx, "", 101); !errors.Is(err, regions.ErrInvalidRegion) {
		t.Fatal("unbounded page accepted")
	}
	if db.db.Dialector.Name() == "postgres" {
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(accept bool) {
				defer wg.Done()
				results <- db.SetRegionAcceptingCreates(ctx, "catalog-b", 1, accept)
			}(i == 0)
		}
		wg.Wait()
		close(results)
		success, conflict := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, regions.ErrRegionConflict) {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("concurrent decisions: success=%d conflict=%d", success, conflict)
		}
	} else {
		if err := migrateSQLiteRegionDirectory(db.db); err != nil {
			t.Fatal(err)
		}
		entry, err := db.GetRegion(ctx, "catalog-a")
		if err != nil || entry.AcceptingCreates || entry.Version != 3 {
			t.Fatalf("repeat migration reset catalog: %+v %v", entry, err)
		}
	}
}
