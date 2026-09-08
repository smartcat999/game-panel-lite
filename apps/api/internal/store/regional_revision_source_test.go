package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func TestRegionalRevisionSource(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "revision-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testRegionalRevisionSource(t, db)
}

func testRegionalRevisionSource(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	org := domain.Organization{ID: "revision-source-owner", Slug: "revision-source-owner"}
	if err := db.CreateOrganization(ctx, &org, "revision-source-user"); err != nil {
		t.Fatal(err)
	}
	request := instances.CreateRequest{OrganizationID: org.ID, Name: "source", RegionID: "source-east", IdempotencyKey: "source-create", Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Configuration: instances.ProtectedConfiguration{KeyID: "test-key", Ciphertext: []byte("opaque")}, Resources: instances.Resources{CPU: 1, MemoryMB: 128}}}
	created, err := db.CreateGlobalServer(ctx, "revision-source-user", request)
	if err != nil {
		t.Fatal(err)
	}
	var row struct{ Payload string }
	if err := db.db.Table("server_outbox").Select("payload").Where("operation_id = ?", created.Operation.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var event instances.RevisionAvailable
	if err := json.Unmarshal([]byte(row.Payload), &event); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.GetRegionalRevision(ctx, "source-east", event)
	if err != nil || snapshot.Revision.ID != created.Revision.ID || snapshot.CurrentSpecGeneration != 1 || snapshot.IntentVersion != 1 || string(snapshot.Revision.Specification.Configuration.Ciphertext) != "opaque" {
		t.Fatalf("revision snapshot mismatch: %+v %v", snapshot, err)
	}
	for _, region := range []string{"", "source-west"} {
		if _, err := db.GetRegionalRevision(ctx, region, event); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unauthorized region read: %v", err)
		}
	}
	for _, mutate := range []func(*instances.RevisionAvailable){
		func(e *instances.RevisionAvailable) { e.EventID = "invented" },
		func(e *instances.RevisionAvailable) { e.OperationID = "invented" },
		func(e *instances.RevisionAvailable) { e.OrganizationID = "foreign" },
		func(e *instances.RevisionAvailable) { e.RevisionID = "foreign" },
		func(e *instances.RevisionAvailable) { e.PlacementEpoch++ },
	} {
		forged := event
		mutate(&forged)
		if result, err := db.GetRegionalRevision(ctx, "source-east", forged); !errors.Is(err, ErrNotFound) || result.Revision.ID != "" {
			t.Fatalf("forged identity leaked revision: %+v %v", result, err)
		}
	}
	revised, err := db.ReviseGlobalServer(ctx, "revision-source-user", instances.ReviseRequest{OrganizationID: org.ID, ServerID: created.Server.ID, ExpectedGeneration: 1, IdempotencyKey: "source-revise", Specification: request.Specification})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = db.GetRegionalRevision(ctx, "source-east", event)
	if err != nil || snapshot.Revision.SpecGeneration != 1 || snapshot.CurrentSpecGeneration != revised.Server.SpecGeneration {
		t.Fatalf("historical revision presented as current: %+v %v", snapshot, err)
	}
	if err := db.db.Table("logical_servers").Where("id = ?", created.Server.ID).Updates(map[string]any{"desired_state": "stopped", "intent_version": 2}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err = db.GetRegionalRevision(ctx, "source-east", event)
	if err != nil || snapshot.DesiredState != "stopped" || snapshot.IntentVersion != 2 {
		t.Fatalf("old event rolled back intent: %+v %v", snapshot, err)
	}
	if err := db.db.Table("server_placements").Where("server_id = ?", created.Server.ID).UpdateColumn("placement_epoch", 2).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetRegionalRevision(ctx, "source-east", event); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired placement read: %v", err)
	}
	if err := db.db.Table("server_placements").Where("server_id = ?", created.Server.ID).Updates(map[string]any{"placement_epoch": 1, "region_id": "source-west"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetRegionalRevision(ctx, "source-east", event); !errors.Is(err, ErrNotFound) {
		t.Fatalf("previous region retained access: %v", err)
	}
	if err := db.db.Table("server_placements").Where("server_id = ?", created.Server.ID).UpdateColumn("region_id", "source-east").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.db.Table("logical_servers").Where("id = ?", created.Server.ID).UpdateColumn("desired_state", "deleted").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetRegionalRevision(ctx, "source-east", event); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted server revision disclosed: %v", err)
	}
}
