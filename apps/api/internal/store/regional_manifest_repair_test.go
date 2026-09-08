package store

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func testRegionalManifestRepair(t *testing.T, db *RegionalStore) {
	t.Helper()
	ctx := context.Background()
	makeSnapshot := func(event instances.RevisionAvailable) regional.RevisionSnapshot {
		return regional.RevisionSnapshot{Event: event, CurrentSpecGeneration: 1, DesiredState: "stopped", IntentVersion: 1, Revision: instances.Revision{ID: event.RevisionID, ServerID: event.ServerID, SpecGeneration: 1, Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 128}, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("legacy")}, Assets: []instances.AssetVersion{{AssetID: "world", Version: "v1"}}}}}
	}
	for _, kind := range []string{"a", "b", "corrupt", "valid", "materialized", "waiting", "oversized"} {
		event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "repair-event-" + kind, OperationID: "repair-" + kind, OrganizationID: "org", ServerID: "server", RevisionID: "revision", RegionID: db.regionID, PlacementEpoch: 1, SpecGeneration: 1}
		if err := db.RecordRevisionNotification(ctx, event); err != nil {
			t.Fatal(err)
		}
		snapshot := makeSnapshot(event)
		if kind == "valid" {
			snapshot.Revision.Specification.Assets = nil
		}
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		status := "revision_fetched"
		if kind == "materialized" {
			status = "materialized"
		}
		if kind == "waiting" {
			status = "awaiting_revision"
		}
		if kind == "corrupt" {
			encoded = []byte("broken")
		}
		if kind == "oversized" {
			encoded = []byte(strings.Repeat(" ", (4<<20)+1))
		}
		if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", event.OperationID).Updates(map[string]any{"snapshot": string(encoded), "status": status, "attempts": 7, "next_attempt_ms": time.Now().Add(time.Hour).UnixMilli()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	preview, err := db.RepairMissingAssetManifests(ctx, "repair-", 2, false)
	if err != nil || preview.Scanned != 2 || len(preview.Candidates) != 2 || preview.Applied || preview.Next != "repair-b" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	var row struct {
		Status, Snapshot string
		Attempts         int64
	}
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", "repair-a").Take(&row).Error; err != nil || row.Status != "revision_fetched" {
		t.Fatal("preview changed task")
	}
	var wg sync.WaitGroup
	counts := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch, err := db.RepairMissingAssetManifests(ctx, "repair-", 100, true)
			if err != nil {
				t.Error(err)
				return
			}
			counts <- len(batch.Candidates)
		}()
	}
	wg.Wait()
	close(counts)
	total := 0
	for n := range counts {
		total += n
	}
	if total != 2 {
		t.Fatalf("concurrent repair count: %d", total)
	}
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", "repair-a").Take(&row).Error; err != nil || row.Status != "awaiting_revision" || row.Attempts != 7 || !strings.Contains(row.Snapshot, "bGVnYWN5") {
		t.Fatal("repair lost original state")
	}
	repeated, err := db.RepairMissingAssetManifests(ctx, "repair-", 100, true)
	if err != nil || len(repeated.Candidates) != 0 || len(repeated.Invalid) != 2 {
		t.Fatalf("repeat repair: %+v %v", repeated, err)
	}
	for _, kind := range []string{"materialized", "waiting"} {
		if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", "repair-"+kind).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		want := "materialized"
		if kind == "waiting" {
			want = "awaiting_revision"
		}
		if row.Status != want || row.Attempts != 7 {
			t.Fatal("repair changed unrelated lifecycle")
		}
	}
	for i := 0; i < 2; i++ {
		claim, err := db.ClaimRevision(ctx, time.Minute)
		if err != nil || claim == nil || !strings.HasPrefix(claim.Event.OperationID, "repair-") {
			t.Fatalf("repaired task not fetchable: %v", err)
		}
		snapshot := makeSnapshot(claim.Event)
		if err := db.SaveRevision(ctx, *claim, snapshot); err == nil {
			t.Fatal("legacy snapshot accepted after repair")
		}
		snapshot.Assets = []assets.PublishedVersion{{OrganizationID: "org", AssetID: "world", Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 1}}
		if err := db.SaveRevision(ctx, *claim, snapshot); err != nil {
			t.Fatal(err)
		}
	}
}
