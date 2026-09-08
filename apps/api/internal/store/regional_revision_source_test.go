package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
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
	asset := assets.PublishedVersion{AssetID: "source-world", OrganizationID: org.ID, Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 4}
	if err := db.PublishAssetVersion(ctx, asset); err != nil {
		t.Fatal(err)
	}
	request.Specification.Assets = []instances.AssetVersion{{AssetID: asset.AssetID, Version: asset.Version}}
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
	if len(snapshot.Assets) != 1 || snapshot.Assets[0] != asset || snapshot.ValidateFor(event) != nil {
		t.Fatal("asset manifest not resolved")
	}
	reference := instances.AssetVersion{AssetID: asset.AssetID, Version: asset.Version}
	resolved, err := db.ResolveRegionalAsset(ctx, "source-east", event, reference)
	if err != nil || resolved != asset {
		t.Fatalf("exact asset authorization: %+v %v", resolved, err)
	}
	unreferenced := asset
	unreferenced.AssetID = "source-unreferenced"
	if err := db.PublishAssetVersion(ctx, unreferenced); err != nil {
		t.Fatal(err)
	}
	if leaked, err := db.ResolveRegionalAsset(ctx, "source-east", event, instances.AssetVersion{AssetID: unreferenced.AssetID, Version: unreferenced.Version}); !errors.Is(err, ErrNotFound) || leaked.AssetID != "" {
		t.Fatal("same-tenant unreferenced asset authorized")
	}
	for _, region := range []string{"", "source-west"} {
		if _, err := db.GetRegionalRevision(ctx, region, event); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unauthorized region read: %v", err)
		}
		if leaked, err := db.ResolveRegionalAsset(ctx, region, event, reference); !errors.Is(err, ErrNotFound) || leaked.AssetID != "" {
			t.Fatal("wrong region authorized asset")
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
		if leaked, err := db.ResolveRegionalAsset(ctx, "source-east", forged, reference); !errors.Is(err, ErrNotFound) || leaked.AssetID != "" {
			t.Fatal("forged event authorized asset")
		}
	}
	newAsset := asset
	newAsset.Version = "v2"
	newAsset.SHA256 = strings.Repeat("b", 64)
	if err := db.PublishAssetVersion(ctx, newAsset); err != nil {
		t.Fatal(err)
	}
	request.Specification.Assets = []instances.AssetVersion{{AssetID: newAsset.AssetID, Version: newAsset.Version}}
	revised, err := db.ReviseGlobalServer(ctx, "revision-source-user", instances.ReviseRequest{OrganizationID: org.ID, ServerID: created.Server.ID, ExpectedGeneration: 1, IdempotencyKey: "source-revise", Specification: request.Specification})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = db.GetRegionalRevision(ctx, "source-east", event)
	if err != nil || snapshot.Revision.SpecGeneration != 1 || snapshot.CurrentSpecGeneration != revised.Server.SpecGeneration {
		t.Fatalf("historical revision presented as current: %+v %v", snapshot, err)
	}
	if len(snapshot.Assets) != 1 || snapshot.Assets[0] != asset {
		t.Fatal("historical manifest changed to latest asset version")
	}
	if resolved, err := db.ResolveRegionalAsset(ctx, "source-east", event, reference); err != nil || resolved != asset {
		t.Fatal("historical asset authorization changed version")
	}
	if leaked, err := db.ResolveRegionalAsset(ctx, "source-east", event, instances.AssetVersion{AssetID: asset.AssetID, Version: "v2"}); !errors.Is(err, ErrNotFound) || leaked.AssetID != "" {
		t.Fatal("new asset version authorized under old revision")
	}
	foreign := domain.Organization{ID: "source-foreign", Slug: "source-foreign"}
	if err := db.CreateOrganization(ctx, &foreign, "source-foreign-user"); err != nil {
		t.Fatal(err)
	}
	foreignAsset := asset
	foreignAsset.AssetID = "source-foreign-asset"
	foreignAsset.OrganizationID = foreign.ID
	if err := db.PublishAssetVersion(ctx, foreignAsset); err != nil {
		t.Fatal(err)
	}
	for i, ref := range []instances.AssetVersion{{AssetID: foreignAsset.AssetID, Version: "v1"}, {AssetID: asset.AssetID, Version: "missing"}} {
		// The legacy internal writer does not authorize asset references. The
		// regional read must independently reject them without a partial snapshot.
		spec := request.Specification
		spec.Assets = []instances.AssetVersion{ref}
		bad, err := db.ReviseGlobalServer(ctx, "revision-source-user", instances.ReviseRequest{OrganizationID: org.ID, ServerID: created.Server.ID, ExpectedGeneration: 2 + int64(i), IdempotencyKey: ref.AssetID, Specification: spec})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.db.Table("server_outbox").Select("payload").Where("operation_id = ?", bad.Operation.ID).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		var badEvent instances.RevisionAvailable
		if err := json.Unmarshal([]byte(row.Payload), &badEvent); err != nil {
			t.Fatal(err)
		}
		if partial, err := db.GetRegionalRevision(ctx, "source-east", badEvent); !errors.Is(err, ErrNotFound) || partial.Revision.ID != "" || len(partial.Assets) != 0 {
			t.Fatalf("unauthorized asset leaked snapshot: %v", err)
		}
		if leaked, err := db.ResolveRegionalAsset(ctx, "source-east", badEvent, ref); !errors.Is(err, ErrNotFound) || leaked.AssetID != "" {
			t.Fatal("invalid legacy asset reference authorized")
		}
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
	if leaked, err := db.ResolveRegionalAsset(ctx, "source-east", event, reference); !errors.Is(err, ErrNotFound) || leaked.AssetID != "" {
		t.Fatal("old placement epoch retained asset authorization")
	}
	if err := db.db.Table("server_placements").Where("server_id = ?", created.Server.ID).Updates(map[string]any{"placement_epoch": 1, "region_id": "source-west"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetRegionalRevision(ctx, "source-east", event); !errors.Is(err, ErrNotFound) {
		t.Fatalf("previous region retained access: %v", err)
	}
	if leaked, err := db.ResolveRegionalAsset(ctx, "source-east", event, reference); !errors.Is(err, ErrNotFound) || leaked.AssetID != "" {
		t.Fatal("previous region retained asset authorization")
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
	if leaked, err := db.ResolveRegionalAsset(ctx, "source-east", event, reference); !errors.Is(err, ErrNotFound) || leaked.AssetID != "" {
		t.Fatal("deleted server retained asset authorization")
	}
}
