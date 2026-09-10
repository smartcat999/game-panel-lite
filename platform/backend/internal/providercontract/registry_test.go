package providercontract

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

func TestSignedManifestDrivesConfigurationAndExactModLock(t *testing.T) {
	store := NewMemoryStore()
	registry := NewRegistry(store, []byte("provider-signing-key-012345678901"))
	manifest, err := registry.Publish(context.Background(), fixtureManifest(), time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(manifest.ManifestDigest, "sha256:") || len(manifest.Signature) != 64 || manifest.ModCatalog == nil || len(manifest.ModCatalog.Signature) != 64 {
		t.Fatalf("unsigned manifest: %#v", manifest)
	}
	verified, err := registry.Verified(context.Background(), manifest.ProviderReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.ValidateRevision(verified,
		map[string]any{"name": "old", "slots": 8, "worldSeed": "fixed"},
		map[string]any{"name": "new", "slots": 16, "worldSeed": "fixed"},
		[]ModSelection{{ModID: "content", Version: "2.0.0"}},
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.ApplyBehavior != ApplyRestart || len(result.ModLock) != 2 || result.ModLock[0].ModID != "content" || !result.ModLock[0].Direct || result.ModLock[1].ModID != "library" || result.ModLock[1].Direct {
		t.Fatalf("result=%#v", result)
	}
	changedCreateOnly := map[string]any{"name": "old", "slots": 8, "worldSeed": "different"}
	if _, err := registry.ValidateRevision(verified, map[string]any{"name": "old", "slots": 8, "worldSeed": "fixed"}, changedCreateOnly, nil, false); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("create-only error=%v", err)
	}
	if _, err := registry.ValidateRevision(verified, nil, map[string]any{"name": "new", "slots": 8, "worldSeed": "fixed", "unknown": true}, nil, true); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("unknown field error=%v", err)
	}
	if _, err := registry.ValidateRevision(verified, nil, map[string]any{"name": "new", "slots": 8, "worldSeed": "fixed"}, []ModSelection{{ModID: "missing", Version: "1.0.0"}}, true); !errors.Is(err, ErrUnresolvedMod) {
		t.Fatalf("unresolved mod error=%v", err)
	}
}

func TestManifestVerificationFailsClosed(t *testing.T) {
	key := []byte("provider-signing-key-012345678901")
	store := NewMemoryStore()
	registry := NewRegistry(store, key)
	manifest, err := registry.Publish(context.Background(), fixtureManifest(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tampered := manifest
	tampered.DisplayName = "tampered"
	store.items[manifest.ProviderReleaseID] = tampered
	if _, err := registry.Verified(context.Background(), manifest.ProviderReleaseID); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("tampered signature error=%v", err)
	}

	invalid := fixtureManifest()
	invalid.ProviderReleaseID = "gpr_invalid"
	field := invalid.ConfigurationSchema.Properties["name"]
	field.Type = "html"
	invalid.ConfigurationSchema.Properties["name"] = field
	if _, err := registry.Publish(context.Background(), invalid, time.Now()); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("unknown field type error=%v", err)
	}
}

func TestSchemaMigrationMustBeExplicit(t *testing.T) {
	manifest := fixtureManifest()
	if mode, err := MigrationMode(manifest, 1, 2); err != nil || mode != "manual" {
		t.Fatalf("mode=%q err=%v", mode, err)
	}
	if _, err := MigrationMode(manifest, 1, 3); !errors.Is(err, ErrUnsupportedMigration) {
		t.Fatalf("migration error=%v", err)
	}
}

func fixtureManifest() Manifest {
	minimum, maximum := 2.0, 32.0
	digest := "sha256:" + strings.Repeat("a", 64)
	libraryDigest := "sha256:" + strings.Repeat("b", 64)
	return Manifest{
		ProviderReleaseID: "gpr_fixture_v2",
		GameKey:           "fixture-game",
		DisplayName:       "Fixture Game",
		ReleaseVersion:    "2.0.0",
		GameVersions:      []string{"1.0.0"},
		SchemaVersion:     2,
		ConfigurationSchema: ConfigurationSchema{
			Type: "object",
			Properties: map[string]Field{
				"name":      {Type: "string", Title: "Name", ApplyBehavior: ApplyHotReload},
				"slots":     {Type: "integer", Title: "Slots", ApplyBehavior: ApplyRestart, Minimum: &minimum, Maximum: &maximum},
				"worldSeed": {Type: "string", Title: "Seed", ApplyBehavior: ApplyCreateOnly},
			},
			Required: []string{"name", "slots", "worldSeed"},
		},
		UISchema: UISchema{
			Sections: []UISection{{ID: "general", Title: "General", Order: 1}},
			Fields: map[string]UIField{
				"name": {Section: "general", Order: 1, Control: "text"}, "slots": {Section: "general", Order: 2, Control: "number"}, "worldSeed": {Section: "general", Order: 3, Control: "text"},
			},
		},
		ListenerRequirements: []contract.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"tcp", "udp"}, InternalPort: 7777, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true}},
		Capabilities:         []string{"configuration", "mods", "console", "logs", "backup", "game-metrics"},
		Metrics:              []Metric{{Key: "players.online", Title: "Players", Unit: "players", Source: "provider-api", FreshnessSeconds: 30, MinimumConfidence: 1}},
		SchemaMigrations:     []Migration{{FromSchemaVersion: 1, ToSchemaVersion: 2, Mode: "manual"}},
		ModCatalog: &ModCatalog{Revision: 1, Entries: []ModCatalogEntry{
			{ModID: "content", DisplayName: "Content", Versions: []ModVersion{{Version: "2.0.0", Digest: digest, Dependencies: []ModDependency{{ModID: "library", Version: "1.0.0"}}}}},
			{ModID: "library", DisplayName: "Library", Versions: []ModVersion{{Version: "1.0.0", Digest: libraryDigest, Dependencies: []ModDependency{}}}},
		}},
	}
}
