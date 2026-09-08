package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func testRegionalDeployments(t *testing.T, db *RegionalStore) {
	ctx := context.Background()
	snapshot := func(key string, generation, intent int64, desired string) regional.RevisionSnapshot {
		event := instances.RevisionAvailable{SchemaVersion: 1, EventID: key, OperationID: key, OrganizationID: "tenant", ServerID: "server", RegionID: db.regionID, RevisionID: fmt.Sprintf("revision-%d", generation), SpecGeneration: generation, PlacementEpoch: 1}
		return regional.RevisionSnapshot{Event: event, CurrentSpecGeneration: generation, IntentVersion: intent, DesiredState: desired, Revision: instances.Revision{ID: event.RevisionID, ServerID: event.ServerID, SpecGeneration: generation, Specification: instances.Specification{ProviderKey: "fixture", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 128}, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("protected")}}}}
	}
	claim := func(s regional.RevisionSnapshot) *regional.AssetClaim {
		t.Helper()
		if err := db.RecordRevisionNotification(ctx, s.Event); err != nil {
			t.Fatal(err)
		}
		fetch, err := db.ClaimRevision(ctx, time.Minute)
		if err != nil || fetch == nil {
			t.Fatalf("fetch: %v", err)
		}
		if err := db.SaveRevision(ctx, *fetch, s); err != nil {
			t.Fatal(err)
		}
		c, err := db.ClaimAssets(ctx, time.Minute)
		if err != nil || c == nil {
			t.Fatalf("assets: %v", err)
		}
		return c
	}
	read := func() regional.Deployment {
		t.Helper()
		var d regional.Deployment
		if err := db.db.Table("regional_deployments").Where("server_id = ? AND placement_epoch = ?", "server", 1).Take(&d).Error; err != nil {
			t.Fatal(err)
		}
		return d
	}
	first := claim(snapshot("first", 1, 1, "running"))
	if err := db.db.Exec("ALTER TABLE regional_deployments ADD CONSTRAINT test_stage_failure CHECK(false) NOT VALID").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteAssets(ctx, *first); err == nil {
		t.Fatal("failed deployment accepted")
	}
	var task struct{ Status string }
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", "first").Take(&task).Error; err != nil || task.Status != "revision_fetched" {
		t.Fatal("asset completion escaped rollback")
	}
	if err := db.db.Exec("ALTER TABLE regional_deployments DROP CONSTRAINT test_stage_failure").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteAssets(ctx, *first); err != nil {
		t.Fatal(err)
	}
	initial := read()
	if initial.ID == "" || initial.Status != "awaiting_authority" || initial.OrganizationID != "tenant" || initial.RevisionOperationID != "first" {
		t.Fatal("invalid pending deployment", initial)
	}
	// Different delivery operations for the same revision converge on one identity.
	cs := make([]*regional.AssetClaim, 8)
	for i := range cs {
		cs[i] = claim(snapshot(fmt.Sprintf("duplicate-%d", i), 1, 1, "running"))
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(cs))
	for _, c := range cs {
		wg.Add(1)
		go func(c *regional.AssetClaim) { defer wg.Done(); errs <- db.CompleteAssets(ctx, *c) }(c)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := read(); got != initial {
		t.Fatal("duplicate replaced identity or source", got)
	}
	newer := claim(snapshot("newer", 3, 3, "stopped"))
	if err := db.CompleteAssets(ctx, *newer); err != nil {
		t.Fatal(err)
	}
	older := claim(snapshot("older", 2, 2, "running"))
	if err := db.CompleteAssets(ctx, *older); err != nil {
		t.Fatal(err)
	}
	current := read()
	if current.ID != initial.ID || current.SpecGeneration != 3 || current.RevisionOperationID != "newer" || current.IntentVersion != 3 || current.DesiredState != "stopped" {
		t.Fatal("late revision rolled back deployment", current)
	}
	for _, mode := range []string{"tenant", "revision", "intent"} {
		s := snapshot("conflict-"+mode, 3, 3, "stopped")
		switch mode {
		case "tenant":
			s.Event.OrganizationID = "other"
		case "revision":
			s.Event.RevisionID = "other"
			s.Revision.ID = "other"
		case "intent":
			s.DesiredState = "running"
		}
		c := claim(s)
		if !errors.Is(db.CompleteAssets(ctx, *c), regional.ErrDeploymentConflict) {
			t.Fatal("conflicting deployment accepted", mode)
		}
	}
	if got := read(); got != current {
		t.Fatal("conflict mutated deployment")
	}
	superseded := snapshot("superseded", 1, 4, "running")
	superseded.Event.ServerID = "superseded"
	superseded.Revision.ServerID = "superseded"
	superseded.CurrentSpecGeneration = 2
	c := claim(superseded)
	if err := db.CompleteAssets(ctx, *c); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.db.Table("regional_deployments").Where("server_id = ?", "superseded").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("known obsolete revision created deployment")
	}
	expired := snapshot("expired", 4, 4, "running")
	c = claim(expired)
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", "expired").Update("asset_lease_until_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(db.CompleteAssets(ctx, *c), regional.ErrAssetClaimLost) {
		t.Fatal("expired completion accepted")
	}
	if got := read(); got != current {
		t.Fatal("expired completion changed desired state")
	}
}
