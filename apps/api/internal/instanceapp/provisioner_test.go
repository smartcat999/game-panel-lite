package instanceapp

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gameconfig"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type provisionerWriter struct{ request instances.CreateRequest }

func (w *provisionerWriter) ReplayCreate(context.Context, string, instances.CreateRequest, []byte) (instances.IntentResult, bool, error) {
	return instances.IntentResult{}, false, nil
}
func (w *provisionerWriter) ReplayRevise(context.Context, string, instances.ReviseRequest, []byte) (instances.IntentResult, bool, error) {
	return instances.IntentResult{}, false, nil
}
func (w *provisionerWriter) Create(_ context.Context, _ string, request instances.CreateRequest, _ []byte) (instances.IntentResult, error) {
	w.request = request
	return instances.IntentResult{Server: instances.Server{ID: "created"}, Operation: instances.Operation{ID: "operation"}}, nil
}
func (w *provisionerWriter) Revise(context.Context, string, instances.ReviseRequest, []byte) (instances.IntentResult, error) {
	return instances.IntentResult{}, errors.New("unexpected revise")
}

type provisionerOffers struct{ offer Offer }

func (o provisionerOffers) ResolveOffer(_ context.Context, id string, version int64) (Offer, error) {
	if id != "tenant-plan" || version != 3 {
		return Offer{}, errors.New("offer unavailable")
	}
	return o.offer, nil
}

type provisionerProviders struct{ registry *provider.Registry }

func (p provisionerProviders) ResolveProvider(key, requested string) (ProviderSpec, bool) {
	gameProvider, ok := p.registry.Get(domain.ProviderKey(key))
	if !ok {
		return ProviderSpec{}, false
	}
	versions := gameProvider.Versions()
	if requested == "" && len(versions) > 0 {
		requested = versions[0]
	}
	for _, version := range versions {
		if version == requested {
			return ProviderSpec{GameVersion: version, ConfigSchemaVersion: gameProvider.CatalogMetadata().ConfigVersion}, true
		}
	}
	return ProviderSpec{}, false
}

func TestProvisionerDerivesInfrastructureFromPlan(t *testing.T) {
	p := terraria.NewVanillaProvider()
	registry, err := provider.NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	writer := &provisionerWriter{}
	intents, err := New(writer, gameconfig.LogicalNormalizer{Providers: registry, MaxBytes: 4096}, &fixtureAdmission{})
	if err != nil {
		t.Fatal(err)
	}
	offer := Offer{ProviderKey: string(p.Key()), RegionID: "east", CPU: 2, MemoryMB: 2048}
	provisioner, err := NewProvisioner(intents, provisionerOffers{offer: offer}, provisionerProviders{registry})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provisioner.Create(context.Background(), "owner", CreateCommand{OrganizationID: "tenant", Name: "server", PlanID: "tenant-plan", PlanVersion: 3, IdempotencyKey: "create", Configuration: []byte(`{}`)})
	if err != nil || result.Server.ID != "created" {
		t.Fatalf("create: %+v %v", result, err)
	}
	if writer.request.RegionID != offer.RegionID || writer.request.Specification.ProviderKey != offer.ProviderKey ||
		writer.request.Specification.Resources.CPU != offer.CPU || writer.request.Specification.Resources.MemoryMB != offer.MemoryMB ||
		writer.request.Specification.GameVersion != p.Versions()[0] || writer.request.Specification.ConfigSchemaVersion != p.CatalogMetadata().ConfigVersion {
		t.Fatalf("untrusted or incomplete derived request: %+v", writer.request)
	}
	badVersion := CreateCommand{OrganizationID: "tenant", Name: "server", PlanID: "tenant-plan", PlanVersion: 3, GameVersion: "not-sold", IdempotencyKey: "bad", Configuration: []byte(`{}`)}
	if _, err := provisioner.Create(context.Background(), "owner", badVersion); !errors.Is(err, ErrInvalidCreateCommand) {
		t.Fatalf("unsupported game version accepted: %v", err)
	}
}
