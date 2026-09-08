package instanceapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gameconfig"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

var errTestAdmission = errors.New("fixture admission denied")

// A deliberately bounded fixture, not a production Region/asset authorizer.
type fixtureAdmission struct{ closed bool }

func (f *fixtureAdmission) CheckCreate(_ context.Context, actor string, r instances.CreateRequest) error {
	if f.closed || actor != "owner" || r.RegionID != "east" || len(r.Specification.Assets) != 0 {
		return errTestAdmission
	}
	return nil
}
func (f *fixtureAdmission) CheckRevise(_ context.Context, actor string, r instances.ReviseRequest) error {
	if f.closed || actor != "owner" || len(r.Specification.Assets) != 0 {
		return errTestAdmission
	}
	return nil
}

func TestApplicationWritesValidatedEncryptedIntents(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "application.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	org := domain.Organization{ID: "tenant", Slug: "tenant"}
	if err := db.CreateOrganization(ctx, &org, "owner"); err != nil {
		t.Fatal(err)
	}
	makeKey := func() []byte {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}
		return key
	}
	protector, err := configprotection.New("config", map[string][]byte{"config": makeKey()}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := configprotection.NewFingerprinter("requests", map[string][]byte{"requests": makeKey()})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := store.NewEncryptedIntentWriter(db, protector, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	p := terraria.NewVanillaProvider()
	registry, err := provider.NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	normalizer := gameconfig.LogicalNormalizer{Providers: registry, MaxBytes: 4096}
	if _, err := New(writer, normalizer, nil); err == nil {
		t.Fatal("missing admission accepted")
	}
	admission := &fixtureAdmission{}
	service, err := New(writer, normalizer, admission)
	if err != nil {
		t.Fatal(err)
	}
	request := instances.CreateRequest{OrganizationID: org.ID, Name: "server", RegionID: "east", IdempotencyKey: "create", Specification: instances.Specification{ProviderKey: string(p.Key()), GameVersion: p.Versions()[0], ConfigSchemaVersion: p.CatalogMetadata().ConfigVersion, Resources: instances.Resources{CPU: 1, MemoryMB: 256}}}
	badRegion := request
	badRegion.RegionID = "west"
	if _, err := service.Create(ctx, "owner", badRegion, []byte(`{}`)); !errors.Is(err, errTestAdmission) {
		t.Fatalf("region gate bypass: %v", err)
	}
	badAssets := request
	badAssets.Specification.Assets = []instances.AssetVersion{{AssetID: "unauthorized", Version: "1"}}
	if _, err := service.Create(ctx, "owner", badAssets, []byte(`{}`)); !errors.Is(err, errTestAdmission) {
		t.Fatalf("asset gate bypass: %v", err)
	}
	if _, err := service.Create(ctx, "owner", request, []byte(`{"port":7777}`)); err == nil {
		t.Fatal("runtime field bypassed provider")
	}
	claimedCipher := request
	claimedCipher.Specification.Configuration = instances.ProtectedConfiguration{KeyID: "untrusted", Ciphertext: []byte("pretend")}
	if _, err := service.Create(ctx, "owner", claimedCipher, []byte(`{}`)); !errors.Is(err, instances.ErrInvalidIntent) {
		t.Fatal("client ciphertext accepted")
	}
	raw := []byte(`{"password":"private","maxPlayers":5}`)
	before := bytes.Clone(raw)
	created, err := service.Create(ctx, "owner", request, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("caller input was cleared")
	}
	replayed, err := service.Create(ctx, "owner", request, []byte(`{ "maxPlayers":5, "password":"private" }`))
	if err != nil || replayed.Operation.ID != created.Operation.ID {
		t.Fatalf("normalized retry changed identity: %v", err)
	}
	revise := instances.ReviseRequest{OrganizationID: org.ID, ServerID: created.Server.ID, ExpectedGeneration: 1, IdempotencyKey: "revise", Specification: request.Specification}
	updated, err := service.Revise(ctx, "owner", revise, []byte(`{"maxPlayers":6}`))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := protector.Open(ctx, instances.ConfigurationBinding{OrganizationID: org.ID, ServerID: created.Server.ID, RevisionID: updated.Revision.ID, SpecGeneration: 2, ProviderKey: request.Specification.ProviderKey, ConfigSchemaVersion: request.Specification.ConfigSchemaVersion}, updated.Revision.Specification.Configuration)
	if err != nil || !bytes.Contains(plaintext, []byte(`"maxPlayers":6`)) || bytes.Contains(plaintext, []byte(`"port"`)) {
		t.Fatalf("normalized revision not stored: %v", err)
	}
	if _, err := service.Revise(ctx, "intruder", revise, []byte(`{}`)); !errors.Is(err, store.ErrWorkspaceWriteDenied) {
		t.Fatal("revise gate bypass")
	}
	admission.closed = true
	replayed, err = service.Create(ctx, "owner", request, raw)
	if err != nil || replayed.Operation.ID != created.Operation.ID || replayed.Server.CurrentRevisionID != updated.Revision.ID {
		t.Fatalf("closed admission blocked original create: %v", err)
	}
	replayed, err = service.Revise(ctx, "owner", revise, []byte(`{"maxPlayers":6}`))
	if err != nil || replayed.Operation.ID != updated.Operation.ID {
		t.Fatalf("closed admission blocked original revision: %v", err)
	}
	if _, err := service.Revise(ctx, "owner", revise, []byte(`{"maxPlayers":7}`)); !errors.Is(err, instances.ErrIdempotencyConflict) {
		t.Fatalf("changed replay bypassed hash: %v", err)
	}
	fresh := request
	fresh.IdempotencyKey = "new-after-close"
	if _, err := service.Create(ctx, "owner", fresh, raw); !errors.Is(err, errTestAdmission) {
		t.Fatalf("new operation bypassed closed admission: %v", err)
	}
	if err := db.RemoveOrganizationMember(ctx, org.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, "owner", request, raw); !errors.Is(err, store.ErrWorkspaceWriteDenied) {
		t.Fatalf("revoked member replayed operation: %v", err)
	}
}
