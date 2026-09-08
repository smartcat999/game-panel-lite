package regional

import (
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func TestRevisionManifestValidation(t *testing.T) {
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "owner", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	first := assets.PublishedVersion{AssetID: "world", OrganizationID: "owner", Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 12}
	second := first
	second.AssetID = "mod"
	base := RevisionSnapshot{Event: event, CurrentSpecGeneration: 1, DesiredState: "running", IntentVersion: 1, Revision: instances.Revision{ID: "revision", ServerID: "server", SpecGeneration: 1, Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 128}, Configuration: instances.ProtectedConfiguration{KeyID: "key", Ciphertext: []byte("opaque")}, Assets: []instances.AssetVersion{{AssetID: "world", Version: "v1"}, {AssetID: "mod", Version: "v1"}}}}, Assets: []assets.PublishedVersion{first, second}}
	if err := base.ValidateFor(event); err != nil {
		t.Fatal(err)
	}
	legacy := base
	legacy.Assets = nil
	if !legacy.NeedsAssetManifest(event) {
		t.Fatal("legacy snapshot was not eligible for authorized refetch")
	}
	wrongEvent := event
	wrongEvent.OrganizationID = "foreign"
	if legacy.NeedsAssetManifest(wrongEvent) {
		t.Fatal("repair accepted a mismatched tenant notification")
	}
	legacy.Revision.SpecGeneration++
	if legacy.NeedsAssetManifest(event) {
		t.Fatal("repair accepted a mismatched revision generation")
	}
	for name, mutate := range map[string]func(*RevisionSnapshot){
		"missing":        func(s *RevisionSnapshot) { s.Assets = nil },
		"extra":          func(s *RevisionSnapshot) { s.Assets = append(s.Assets, first) },
		"duplicate":      func(s *RevisionSnapshot) { s.Assets[1] = first },
		"foreign tenant": func(s *RevisionSnapshot) { s.Assets[0].OrganizationID = "foreign" },
		"wrong version":  func(s *RevisionSnapshot) { s.Assets[0].Version = "v2" },
		"wrong asset":    func(s *RevisionSnapshot) { s.Assets[0].AssetID = "backup" },
		"invalid digest": func(s *RevisionSnapshot) { s.Assets[0].SHA256 = "invalid" },
		"negative size":  func(s *RevisionSnapshot) { s.Assets[0].SizeBytes = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			s := base
			s.Assets = append([]assets.PublishedVersion(nil), base.Assets...)
			mutate(&s)
			if s.ValidateFor(event) == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}
