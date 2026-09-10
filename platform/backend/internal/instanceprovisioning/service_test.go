package instanceprovisioning

import (
	"context"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

type captureDelivery struct{ command deliverycontrol.CreateCommand }

func (d *captureDelivery) Create(_ context.Context, command deliverycontrol.CreateCommand, _ time.Time) (deliverycontrol.Instance, deliverycontrol.Operation, error) {
	d.command = command
	return deliverycontrol.Instance{ID: "lin_fixture"}, deliverycontrol.Operation{ID: "op_fixture"}, nil
}

func TestCreateIsFullyDerivedFromVerifiedProviderContract(t *testing.T) {
	registry := providercontract.NewRegistry(providercontract.NewMemoryStore(), []byte("provider-signing-key-012345678901"))
	manifest, err := registry.Publish(context.Background(), createManifest(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	delivery := &captureDelivery{}
	service := New(registry, delivery)
	if _, _, err := service.Create(context.Background(), Command{WorkspaceID: "ws_one", Name: "server", ProviderReleaseID: manifest.ProviderReleaseID, GameVersion: "1.0", Configuration: map[string]any{"slots": float64(8)}, QuoteID: "quo_one", IdempotencyKey: "create-one"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if delivery.command.SchemaVersion != manifest.SchemaVersion || len(delivery.command.ListenerRequirements) != 1 || delivery.command.ListenerRequirements[0].InternalPort != 7000 {
		t.Fatalf("derived command=%#v", delivery.command)
	}
}

func createManifest() providercontract.Manifest {
	minimum, maximum := 1.0, 16.0
	return providercontract.Manifest{ProviderReleaseID: "gpr_fixture", GameKey: "fixture", DisplayName: "Fixture", ReleaseVersion: "1", GameVersions: []string{"1.0"}, SchemaVersion: 1, ConfigurationSchema: providercontract.ConfigurationSchema{Type: "object", Properties: map[string]providercontract.Field{"slots": {Type: "integer", Title: "Slots", ApplyBehavior: providercontract.ApplyRestart, Minimum: &minimum, Maximum: &maximum}}, Required: []string{"slots"}}, UISchema: providercontract.UISchema{Sections: []providercontract.UISection{{ID: "general", Title: "General"}}, Fields: map[string]providercontract.UIField{"slots": {Section: "general", Control: "number"}}}, ListenerRequirements: []contract.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"udp"}, InternalPort: 7000, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true}}, Capabilities: []string{"configuration"}, Metrics: []providercontract.Metric{}, SchemaMigrations: []providercontract.Migration{}}
}
