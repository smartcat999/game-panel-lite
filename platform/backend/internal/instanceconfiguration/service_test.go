package instanceconfiguration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

type memoryDrafts struct{ draft Draft }

func (s *memoryDrafts) Create(_ context.Context, draft Draft) error { s.draft = draft; return nil }
func (s *memoryDrafts) Save(_ context.Context, draft Draft) error   { s.draft = draft; return nil }
func (s *memoryDrafts) ByID(_ context.Context, workspaceID, instanceID, draftID string) (Draft, error) {
	if s.draft.ID != draftID || s.draft.WorkspaceID != workspaceID || s.draft.LogicalInstanceID != instanceID {
		return Draft{}, sql.ErrNoRows
	}
	return s.draft, nil
}

type configurationDelivery struct {
	instance deliverycontrol.Instance
	revision deliverycontrol.Revision
	applied  deliverycontrol.ApplyRevisionCommand
}

func (d *configurationDelivery) Instance(context.Context, string, string) (deliverycontrol.Instance, error) {
	return d.instance, nil
}
func (d *configurationDelivery) Revision(context.Context, string, string, string) (deliverycontrol.Revision, error) {
	return d.revision, nil
}
func (d *configurationDelivery) ApplyRevision(_ context.Context, command deliverycontrol.ApplyRevisionCommand, _ time.Time) (deliverycontrol.Revision, deliverycontrol.Operation, error) {
	d.applied = command
	return deliverycontrol.Revision{ID: "rev_two"}, deliverycontrol.Operation{ID: "op_apply"}, nil
}

func TestInvalidDraftAutosavesButCannotApply(t *testing.T) {
	registry := providercontract.NewRegistry(providercontract.NewMemoryStore(), []byte("provider-signing-key-012345678901"))
	manifest, err := registry.Publish(context.Background(), configurationManifest(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	delivery := &configurationDelivery{instance: deliverycontrol.Instance{ID: "lin_one", WorkspaceID: "ws_one", ProviderReleaseID: manifest.ProviderReleaseID, GameVersion: "1.0", InstanceRevisionID: "rev_one"}, revision: deliverycontrol.Revision{ID: "rev_one", WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", ProviderReleaseID: manifest.ProviderReleaseID, GameVersion: "1.0", SchemaVersion: 1, Configuration: map[string]any{"slots": float64(8)}}}
	store := &memoryDrafts{}
	service := New(registry, delivery, store)
	draft, err := service.CreateDraft(context.Background(), "ws_one", "lin_one", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	draft, err = service.SaveDraft(context.Background(), SaveCommand{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", DraftID: draft.ID, SchemaVersion: 1, Values: map[string]any{"slots": float64(100)}}, time.Now().UTC())
	if err != nil || len(draft.ValidationErrors) != 1 {
		t.Fatalf("draft=%#v err=%v", draft, err)
	}
	if _, _, err := service.Apply(context.Background(), ApplyCommand{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", DraftID: draft.ID, IdempotencyKey: "apply-one"}, time.Now().UTC()); err != ErrInvalidDraft {
		t.Fatalf("apply error=%v", err)
	}
	draft, err = service.SaveDraft(context.Background(), SaveCommand{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", DraftID: draft.ID, SchemaVersion: 1, Values: map[string]any{"slots": float64(12)}}, time.Now().UTC())
	if err != nil || len(draft.ValidationErrors) != 0 {
		t.Fatalf("valid draft=%#v err=%v", draft, err)
	}
	if _, _, err := service.Apply(context.Background(), ApplyCommand{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", DraftID: draft.ID, IdempotencyKey: "apply-two"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if delivery.applied.ApplyBehavior != string(providercontract.ApplyRestart) {
		t.Fatalf("apply command=%#v", delivery.applied)
	}
}

func configurationManifest() providercontract.Manifest {
	minimum, maximum := 1.0, 16.0
	return providercontract.Manifest{ProviderReleaseID: "gpr_fixture", GameKey: "fixture", DisplayName: "Fixture", ReleaseVersion: "1", GameVersions: []string{"1.0"}, SchemaVersion: 1, ConfigurationSchema: providercontract.ConfigurationSchema{Type: "object", Properties: map[string]providercontract.Field{"slots": {Type: "integer", Title: "Slots", ApplyBehavior: providercontract.ApplyRestart, Minimum: &minimum, Maximum: &maximum}}, Required: []string{"slots"}}, UISchema: providercontract.UISchema{Sections: []providercontract.UISection{{ID: "general", Title: "General"}}, Fields: map[string]providercontract.UIField{"slots": {Section: "general", Control: "number"}}}, ListenerRequirements: []contract.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"udp"}, InternalPort: 7000, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true}}, Capabilities: []string{"configuration"}, Metrics: []providercontract.Metric{}, SchemaMigrations: []providercontract.Migration{}}
}
