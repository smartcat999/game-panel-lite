package regional

import (
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func TestAssetSourceBinding(t *testing.T) {
	e := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "org", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	ref := instances.AssetVersion{AssetID: "world", Version: "v1"}
	s := AssetSourceSnapshot{Event: e, Asset: assets.PublishedVersion{AssetID: "world", OrganizationID: "org", Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 1}, Replica: assets.Replica{ID: "replica", AssetID: "world", AssetVersion: "v1", RegionID: "west", StorageID: "store", Available: true, Version: 2}}
	if err := s.ValidateFor(e, ref, "replica", 2); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*AssetSourceSnapshot){func(s *AssetSourceSnapshot) { s.Event.PlacementEpoch++ }, func(s *AssetSourceSnapshot) { s.Asset.OrganizationID = "foreign" }, func(s *AssetSourceSnapshot) { s.Replica.AssetID = "other" }, func(s *AssetSourceSnapshot) { s.Replica.AssetVersion = "v2" }, func(s *AssetSourceSnapshot) { s.Replica.Version++ }, func(s *AssetSourceSnapshot) { s.Replica.Available = false }, func(s *AssetSourceSnapshot) { s.Replica.StorageID = "https://host" }, func(s *AssetSourceSnapshot) { s.Asset.SHA256 = "invalid" }} {
		bad := s
		mutate(&bad)
		if bad.ValidateFor(e, ref, "replica", 2) == nil {
			t.Fatal("invalid source binding accepted")
		}
	}
}
