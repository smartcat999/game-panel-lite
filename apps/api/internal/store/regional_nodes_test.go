package store

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func testRegionalNodes(t *testing.T, db *RegionalStore, dsn string) {
	ctx := context.Background()
	config := regional.NodeConfiguration{ID: "node-m", Name: "node", Architecture: "amd64", CPU: 8, MemoryMB: 8192, Schedulable: true}
	concurrent := func(expected int64) {
		t.Helper()
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := db.ConfigureRegionalNode(ctx, config, expected); errs <- err }()
		}
		wg.Wait()
		close(errs)
		success := 0
		for err := range errs {
			if err == nil {
				success++
			} else if !errors.Is(err, regional.ErrNodeVersionConflict) {
				t.Fatal(err)
			}
		}
		if success != 1 {
			t.Fatalf("concurrent configuration successes: %d", success)
		}
	}
	concurrent(0)
	config.Name = "updated"
	config.CPU = 4
	config.MemoryMB = 4096
	config.Schedulable = false
	concurrent(1)
	nodes, err := db.ListRegionalNodes(ctx, "", 1)
	if err != nil || len(nodes) != 1 || nodes[0].Version != 2 || nodes[0].NodeConfiguration != config {
		t.Fatalf("configuration not persisted: %+v %v", nodes, err)
	}
	if _, err := db.ConfigureRegionalNode(ctx, config, 0); !errors.Is(err, regional.ErrNodeVersionConflict) {
		t.Fatal("create overwrote node")
	}
	if _, err := db.ConfigureRegionalNode(ctx, config, 1); !errors.Is(err, regional.ErrNodeVersionConflict) {
		t.Fatal("stale write accepted")
	}
	for _, mutate := range []func(*regional.NodeConfiguration){
		func(n *regional.NodeConfiguration) { n.ID = "" }, func(n *regional.NodeConfiguration) { n.Name = " " }, func(n *regional.NodeConfiguration) { n.Architecture = "amd64\n" },
		func(n *regional.NodeConfiguration) { n.CPU = 0 }, func(n *regional.NodeConfiguration) { n.CPU = math.Inf(1) }, func(n *regional.NodeConfiguration) { n.CPU = math.NaN() }, func(n *regional.NodeConfiguration) { n.MemoryMB = 0 },
	} {
		bad := config
		mutate(&bad)
		if _, err := db.ConfigureRegionalNode(ctx, bad, 2); !errors.Is(err, regional.ErrInvalidNode) {
			t.Fatal("invalid configuration accepted")
		}
	}
	for _, version := range []int64{-1, math.MaxInt64} {
		if _, err := db.ConfigureRegionalNode(ctx, config, version); !errors.Is(err, regional.ErrInvalidNode) {
			t.Fatal("invalid version accepted")
		}
	}
	reopened, err := OpenRegionalPostgres(dsn, db.regionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.ListRegionalNodes(ctx, "", 1)
	if err != nil || len(persisted) != 1 || persisted[0] != nodes[0] {
		t.Fatal("node changed after failed update or reopen")
	}
	for _, id := range []string{"node-a", "node-b", "node-c"} {
		c := config
		c.ID = id
		if _, err := db.ConfigureRegionalNode(ctx, c, 0); err != nil {
			t.Fatal(err)
		}
	}
	page, err := db.ListRegionalNodes(ctx, "", 2)
	if err != nil || len(page) != 2 || page[0].ID != "node-a" || page[1].ID != "node-b" {
		t.Fatal("invalid first page")
	}
	page, err = db.ListRegionalNodes(ctx, page[1].ID, 2)
	if err != nil || len(page) != 2 || page[0].ID != "node-c" || page[1].ID != "node-m" {
		t.Fatal("invalid cursor page")
	}
	page, err = db.ListRegionalNodes(ctx, page[1].ID, 2)
	if err != nil || len(page) != 0 {
		t.Fatal("last page not empty")
	}
	for _, limit := range []int{0, 201} {
		if _, err := db.ListRegionalNodes(ctx, "", limit); !errors.Is(err, regional.ErrInvalidNode) {
			t.Fatal("unbounded list accepted")
		}
	}
	testRegionalNodeSessions(t, db)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := db.ConfigureRegionalNode(cancelled, config, 2); err == nil {
		t.Fatal("cancelled write accepted")
	}
}
